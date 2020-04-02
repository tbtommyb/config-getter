package controller

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	api_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type Controller struct {
	logger       *logrus.Entry
	clientset    kubernetes.Interface
	queue        workqueue.RateLimitingInterface
	informer     cache.SharedIndexInformer
	eventHandler Handler
}

const maxRetries = 5

func New(clientset kubernetes.Interface) *Controller {
	logger := logrus.WithField("pkg", "config-getter")
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// TODO: is it possible to filter CMs here by annotation?
	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return clientset.CoreV1().ConfigMaps(meta_v1.NamespaceAll).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return clientset.CoreV1().ConfigMaps(meta_v1.NamespaceAll).Watch(options)
			},
		},
		&api_v1.ConfigMap{},
		0, //Skip resync
		cache.Indexers{},
	)
	// TODO: only queue if CM doesn't have specified key in data
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			logger.Infof("Processing add for %s", key)
			if err == nil {
				queue.Add(key)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			oldCM := old.(*api_v1.ConfigMap)
			oldAnnotation, present := oldCM.GetAnnotations()["x-k8s-io/curl-me-that"]
			if present {
				newCM := new.(*api_v1.ConfigMap)
				newAnnotation, _ := newCM.GetAnnotations()["x-k8s-io/curl-me-that"]
				if oldAnnotation == newAnnotation {
					logger.Infof("old %s and new %s are same, skipping", oldAnnotation, newAnnotation)
					return
				}
			}
			key, err := cache.MetaNamespaceKeyFunc(old)
			logger.Infof("Processing update for %s", key)
			if err == nil {
				queue.Add(key)
			}
		},
	})

	controller := &Controller{
		logger:       logger,
		clientset:    clientset,
		queue:        queue,
		informer:     informer,
		eventHandler: &ConfigGetter{logger, clientset},
	}

	return controller
}

// Run will start the controller.
// StopCh channel is used to send interrupt signal to stop it.
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.logger.Info("Starting controller")

	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	c.logger.Info("Controller synced and ready")

	wait.Until(c.runWorker, time.Second, stopCh)
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

// processNextItem deals with one key off the queue.  It returns false
// when it's time to quit.
func (c *Controller) processNextItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}

	defer c.queue.Done(key)

	err := c.processItem(key.(string))

	if err == nil {
		c.queue.Forget(key)
	} else if c.queue.NumRequeues(key) < maxRetries {
		c.logger.Errorf("Error processing %s (will retry): %v", key, err)
		c.queue.AddRateLimited(key)
	} else {
		c.logger.Errorf("Error processing %s (giving up): %v", key, err)
		c.queue.Forget(key)
		utilruntime.HandleError(err)
	}

	return true
}

func (c *Controller) processItem(key string) error {
	obj, exists, err := c.informer.GetIndexer().GetByKey(key)
	if err != nil {
		return fmt.Errorf("Error fetching object with key %s from store: %v", key, err)
	}
	if !exists {
		return nil
	}

	c.eventHandler.Process(obj)
	return nil
}

// HasSynced is required for the cache.Controller interface.
func (c *Controller) HasSynced() bool {
	return c.informer.HasSynced()
}

type Handler interface {
	Process(interface{})
}

type ConfigGetter struct {
	logger    *logrus.Entry
	clientset kubernetes.Interface
}

func (cg *ConfigGetter) Process(obj interface{}) {
	cm := obj.(*api_v1.ConfigMap)
	cg.logger.Infof("Processing %s\n", cm.GetName())

	annotations := cm.GetAnnotations()
	curl, ok := annotations["x-k8s-io/curl-me-that"]
	if !ok {
		cg.logger.Infof("No matching annotation found for %s", cm.GetName())
		return
	}
	key, url := parseAnnotation(curl)
	validatedURL, err := validateUrl(url)
	cg.logger.Infof("validatedURL %s\n", validatedURL)
	if err != nil {
		cg.logger.Errorf("Could not parse URL %s: %s", url, err.Error())
		return
	}
	resp, err := http.Get(validatedURL)
	if err != nil {
		// TODO: think about this. Store in CM somewhere
		cg.logger.Warnf("Failed to fetch URL %s", url)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		cg.logger.Errorf(err.Error())
		return
	}
	bodyString := string(body)
	if data, exists := cm.Data[key]; exists {
		if data == bodyString {
			cg.logger.Info("No need to change key")
			return
		}
		cg.logger.Warnf("Overwriting key %s for %s", key, cm.GetName())
	}

	updated := cm.DeepCopy()
	data := updated.Data
	if data == nil {
		updated.Data = make(map[string]string)
	}

	updated.Data[key] = bodyString
	_, err = cg.clientset.CoreV1().ConfigMaps(cm.GetNamespace()).Update(updated)
	if err != nil {
		cg.logger.Errorf("Error updating ConfigMap %s: %#v", updated.GetName(), err)
	}
}

func parseAnnotation(annotation string) (string, string) {
	x := strings.Split(annotation, "=")
	return x[0], x[1]
}

func validateUrl(path string) (string, error) {
	u, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	fmt.Printf("%#v\n", u)
	if u.Path == "" {
		return "", errors.New("no URL path")
	}
	if u.Scheme == "" {
		u.Scheme = "http://"
	}

	return u.Scheme + u.Path, nil
}
