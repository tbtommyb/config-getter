package controller

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	api_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type Handler interface {
	Process(*api_v1.ConfigMap) (*api_v1.ConfigMap, error)
}

type Controller struct {
	logger       *logrus.Entry
	Clientset    kubernetes.Interface
	queue        workqueue.RateLimitingInterface
	informer     cache.SharedIndexInformer
	eventHandler Handler
	HasSynced    cache.InformerSynced
	recorder     record.EventRecorder
}

const maxRetries = 5

func New(Clientset kubernetes.Interface, handler Handler) *Controller {
	logger := logrus.WithField("pkg", "config-getter")
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// TODO: is it possible to filter CMs here by annotation?
	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return Clientset.CoreV1().ConfigMaps(meta_v1.NamespaceAll).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return Clientset.CoreV1().ConfigMaps(meta_v1.NamespaceAll).Watch(options)
			},
		},
		&api_v1.ConfigMap{},
		0, //Skip resync
		cache.Indexers{},
	)
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				logger.Infof("Processing add for %s", key)
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
					return
				}
			}
			key, err := cache.MetaNamespaceKeyFunc(old)
			if err == nil {
				logger.Infof("Processing update for %s", key)
				queue.Add(key)
			}
		},
	})

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logger.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: Clientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, api_v1.EventSource{Component: "config-getter-controller"})

	controller := &Controller{
		logger:       logger,
		Clientset:    Clientset,
		queue:        queue,
		informer:     informer,
		eventHandler: handler,
		HasSynced:    informer.HasSynced,
		recorder:     recorder,
	}

	return controller
}

func (c *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.logger.Info("Starting config-getter controller")

	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Time out waiting for cache sync"))
		return
	}

	c.logger.Info("Controller synced and ready")

	wait.Until(c.runWorker, time.Second, stopCh)
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

func (c *Controller) processNextItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	obj, exists, err := c.informer.GetIndexer().GetByKey(key.(string))
	if err != nil {
		c.logger.Errorf("get by key error %s: %w", key, err)
		return true
	}
	if !exists {
		c.logger.Errorf("key %s does not exist in store", key)
		return true
	}

	cm := obj.(*api_v1.ConfigMap)
	err = c.processItem(cm)

	if err == nil {
		c.queue.Forget(key)
	} else if c.queue.NumRequeues(key) < maxRetries {
		c.recorder.Eventf(cm, api_v1.EventTypeWarning, "GetFailure", "Error processing %s (will retry): %v", key, err)
		c.queue.AddRateLimited(key)
	} else {
		c.recorder.Eventf(cm, api_v1.EventTypeWarning, "GetFailure", "Error processing %s (giving up): %v", key, err)
		c.queue.Forget(key)
		utilruntime.HandleError(err)
	}

	return true
}

func (c *Controller) processItem(cm *api_v1.ConfigMap) error {
	updated, err := c.eventHandler.Process(cm)
	if err != nil {
		return err
	}
	if updated == nil {
		return nil
	}
	_, err = c.Clientset.CoreV1().ConfigMaps(updated.GetNamespace()).Update(updated)
	return err
}
