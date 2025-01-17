package controller

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	api_v1 "k8s.io/api/core/v1"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// Handler specifies the interface for processing each ConfigMap
type Handler interface {
	Process(*api_v1.ConfigMap) (*api_v1.ConfigMap, error)
}

// Logger defines the required logging operations
type Logger interface {
	Infof(string, ...interface{})
	Info(...interface{})
	Errorf(string, ...interface{})
}

// Controller specifies the core controller
type Controller struct {
	logger       Logger
	Clientset    kubernetes.Interface
	queue        workqueue.RateLimitingInterface
	informer     cache.SharedIndexInformer
	eventHandler Handler
	HasSynced    cache.InformerSynced
	recorder     record.EventRecorder
}

const maxRetries = 5
const controllerAnnotation = "x-k8s-io/curl-me-that"

// New creates a new controller
func New(Clientset kubernetes.Interface, handler Handler, informer cache.SharedIndexInformer, logger Logger, recorder record.EventRecorder) *Controller {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

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
			newCM := new.(*api_v1.ConfigMap)

			if oldCM.ResourceVersion == newCM.ResourceVersion {
				return
			}

			if _, present := newCM.GetAnnotations()[controllerAnnotation]; !present {
				return
			}

			key, err := cache.MetaNamespaceKeyFunc(old)
			if err == nil {
				logger.Infof("Processing update for %s", key)
				queue.Add(key)
			}
		},
	})

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

// Run starts the controller and runs until a stop signal is received
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.logger.Info("starting config-getter-controller")

	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("time out waiting for cache sync"))
		return
	}

	c.logger.Info("config-getter-controller synced and ready")

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
		c.recorder.Eventf(cm, api_v1.EventTypeWarning, "GetFailure", "Error (will retry): %v", err)
		c.queue.AddRateLimited(key)
	} else {
		c.recorder.Eventf(cm, api_v1.EventTypeWarning, "GetFailure", "Error (giving up): %v", err)
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
