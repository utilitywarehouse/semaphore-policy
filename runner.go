package main

import (
	"fmt"
	"time"

	calicoClient "github.com/projectcalico/libcalico-go/lib/clientv3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/utilitywarehouse/kube-policy-semaphore/kube"
	"github.com/utilitywarehouse/kube-policy-semaphore/log"
)

type Runner struct {
	podWatcher            *kube.PodWatcher
	nsStore               NetworkSetStore
	nameAnnotation        string
	canSync               bool
	fullStoreResyncPeriod time.Duration
	stop                  chan struct{}
}

func newRunner(client calicoClient.Interface, watchClient kubernetes.Interface, cluster, labelSelector, nameAnnotaion string, fullStoreResyncPeriod, podResyncPeriod time.Duration) *Runner {
	runner := &Runner{
		nsStore:               newNetworkSetStore(cluster, client),
		nameAnnotation:        nameAnnotaion,
		canSync:               false,
		fullStoreResyncPeriod: fullStoreResyncPeriod,
		stop:                  make(chan struct{}),
	}

	podWatcher := kube.NewPodWatcher(
		watchClient,
		podResyncPeriod,
		runner.PodEventHandler,
		labelSelector,
	)
	runner.podWatcher = podWatcher
	runner.podWatcher.Init()

	return runner
}

func (r *Runner) Start() error {
	go r.podWatcher.Run()
	go r.nsStore.RunSyncLoop()
	// wait for node watcher to sync. TODO: atm dummy and could run forever
	// if node cache fails to sync
	stopCh := make(chan struct{})
	if ok := cache.WaitForNamedCacheSync("podWatcher", stopCh, r.podWatcher.HasSynced); !ok {
		return fmt.Errorf("failed to wait for pods cache to sync")
	}
	r.canSync = true
	r.nsStore.fullSyncQueue <- struct{}{}
	return nil
}

func (r *Runner) Run() {
	ticker := time.NewTicker(r.fullStoreResyncPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.nsStore.fullSyncQueue <- struct{}{}
		case <-r.stop:
			log.Logger.Debug("Stopping runner")
			return
		}
	}
}

func (r *Runner) Healthy() bool {
	return r.podWatcher.Healthy()
}

func (r *Runner) Stop() {
	r.stop <- struct{}{}
	r.nsStore.stop <- struct{}{}
}

func (r *Runner) PodEventHandler(eventType watch.EventType, old *v1.Pod, new *v1.Pod) {
	switch eventType {
	case watch.Added:
		log.Logger.Debug("Received add event", "pod", new.Name, "ip", new.Status.PodIP)
		r.onPodAdd(new)
	case watch.Modified:
		log.Logger.Debug("Received modify event", "new_pod", new.Name, "new_pod_ip", new.Status.PodIP, "old_pod", old.Name, "old_pod_ip", old.Status.PodIP)
		r.onPodModify(old, new)
	case watch.Deleted:
		log.Logger.Debug("Received delete event", "old_pod", old.Name, "old_pod_ip", old.Status.PodIP)
		r.onPodDelete(old)
	default:
		log.Logger.Info(
			"Unknown endpoints event received: %v",
			eventType,
		)
	}
}

func (r *Runner) onPodAdd(pod *v1.Pod) {
	name, ok := pod.Annotations[r.nameAnnotation]
	if !ok {
		log.Logger.Error("Annotation not found for labelled pod", "anno", r.nameAnnotation, "pod", pod.Name)
		return
	}
	if pod.Status.PodIP != "" {
		r.nsStore.AddNet(name, pod.Namespace, fmt.Sprintf("%s/32", pod.Status.PodIP))
		if r.canSync {
			r.nsStore.EnqueueNetSetSync(name, pod.Namespace)
		}
	}
}

func (r *Runner) onPodModify(old *v1.Pod, new *v1.Pod) {
	name, ok := new.Annotations[r.nameAnnotation]
	if !ok {
		log.Logger.Error("Annotation not found for labelled pod", "anno", r.nameAnnotation, "pod", new.Name)
		return
	}
	altered := false
	if new.Status.PodIP != "" && new.Status.PodIP != old.Status.PodIP {
		r.nsStore.AddNet(name, new.Namespace, fmt.Sprintf("%s/32", new.Status.PodIP))
		if old.Status.PodIP != "" {
			r.nsStore.DeleteNet(name, new.Namespace, fmt.Sprintf("%s/32", old.Status.PodIP))
		}
		altered = true
	}
	if new.Status.PodIP == "" && old.Status.PodIP != "" {
		r.nsStore.DeleteNet(name, new.Namespace, fmt.Sprintf("%s/32", old.Status.PodIP))
		altered = true
	}
	if altered {
		if r.canSync {
			r.nsStore.EnqueueNetSetSync(name, new.Namespace)
		}
	}

}

func (r *Runner) onPodDelete(pod *v1.Pod) {
	name, ok := pod.Annotations[r.nameAnnotation]
	if !ok {
		log.Logger.Error("Annotation not found for labelled pod", "anno", r.nameAnnotation, "pod", pod.Name)
		return
	}
	if pod.Status.PodIP != "" {
		r.nsStore.DeleteNet(name, pod.Namespace, fmt.Sprintf("%s/32", pod.Status.PodIP))
		if r.canSync {
			r.nsStore.EnqueueNetSetSync(name, pod.Namespace)
		}
	}
}
