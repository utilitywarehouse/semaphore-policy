package kube

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/utilitywarehouse/semaphore-policy/log"
	"github.com/utilitywarehouse/semaphore-policy/metrics"
)

// PodEventHandler is the function to handle new events
type PodEventHandler = func(eventType watch.EventType, old *v1.Pod, new *v1.Pod)

// PodWatcher has a watch on the clients pods
type PodWatcher struct {
	ctx           context.Context
	client        kubernetes.Interface
	resyncPeriod  time.Duration
	stopChannel   chan struct{}
	store         cache.Store
	controller    cache.Controller
	eventHandler  PodEventHandler
	labelSelector string
	ListHealthy   bool
	WatchHealthy  bool
}

// NewPodWatcher returns a new pod wathcer.
func NewPodWatcher(client kubernetes.Interface, resyncPeriod time.Duration, handler PodEventHandler, labelSelector string) *PodWatcher {
	return &PodWatcher{
		ctx:           context.Background(),
		client:        client,
		resyncPeriod:  resyncPeriod,
		stopChannel:   make(chan struct{}),
		eventHandler:  handler,
		labelSelector: labelSelector,
	}
}

// Init sets up the list, watch functions and the cache.
func (pw *PodWatcher) Init() {
	listWatch := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.LabelSelector = pw.labelSelector
			l, err := pw.client.CoreV1().Pods(metav1.NamespaceAll).List(pw.ctx, options)
			if err != nil {
				log.Logger.Error("pw: list error", "err", err)
				pw.ListHealthy = false
				metrics.IncPodWatcherFailures("list")
			} else {
				pw.ListHealthy = true
			}
			return l, err
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.LabelSelector = pw.labelSelector
			w, err := pw.client.CoreV1().Pods(metav1.NamespaceAll).Watch(pw.ctx, options)
			if err != nil {
				log.Logger.Error("pw: watch error", "err", err)
				pw.WatchHealthy = false
				metrics.IncPodWatcherFailures("watch")
			} else {
				pw.WatchHealthy = true
			}
			return w, err
		},
	}
	eventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pw.eventHandler(watch.Added, nil, obj.(*v1.Pod))
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			pw.eventHandler(watch.Modified, oldObj.(*v1.Pod), newObj.(*v1.Pod))
		},
		DeleteFunc: func(obj interface{}) {
			pw.eventHandler(watch.Deleted, obj.(*v1.Pod), nil)
		},
	}
	pw.store, pw.controller = cache.NewInformer(listWatch, &v1.Pod{}, pw.resyncPeriod, eventHandler)
}

// Run will not return unless writting in the stop channel
func (pw *PodWatcher) Run() {
	log.Logger.Info("starting pod watcher")
	// Running controller will block until writing on the stop channel.
	pw.controller.Run(pw.stopChannel)
	log.Logger.Info("stopped pod watcher")
}

// Stop stop the watcher via the respective channel
func (pw *PodWatcher) Stop() {
	log.Logger.Info("stopping pod watcher")
	close(pw.stopChannel)
}

// HasSynced calls controllers HasSync method to determine whether the watcher
// cache is synced.
func (pw *PodWatcher) HasSynced() bool {
	return pw.controller.HasSynced()
}

// List lists all pods from the store
func (pw *PodWatcher) List() ([]*v1.Pod, error) {
	var svcs []*v1.Pod
	for _, obj := range pw.store.List() {
		svc, ok := obj.(*v1.Pod)
		if !ok {
			return nil, fmt.Errorf("unexpected object in store: %+v", obj)
		}
		svcs = append(svcs, svc)
	}
	return svcs, nil
}

// Healthy is true when both list and watch handlers are running without errors.
func (pw *PodWatcher) Healthy() bool {
	if pw.ListHealthy && pw.WatchHealthy {
		return true
	}
	return false
}
