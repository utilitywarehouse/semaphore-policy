package main

import (
	"fmt"
	"time"

	calicoClientset "github.com/projectcalico/api/pkg/client/clientset_generated/clientset"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/utilitywarehouse/semaphore-policy/kube"
	"github.com/utilitywarehouse/semaphore-policy/log"
)

type Runner struct {
	podWatcher *kube.PodWatcher
	nsStore    NetworkSetStore
	canSync    bool
	stop       chan struct{}
}

func newRunner(client *calicoClientset.Clientset, watchClient kubernetes.Interface, cluster string, podResyncPeriod time.Duration) *Runner {
	runner := &Runner{
		nsStore: newNetworkSetStore(cluster, client),
		canSync: false,
		stop:    make(chan struct{}),
	}

	podWatcher := kube.NewPodWatcher(
		watchClient,
		podResyncPeriod,
		runner.PodEventHandler,
		labelNetSetName,
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

func (r *Runner) Healthy() bool {
	return r.podWatcher.Healthy()
}

func (r *Runner) Stop() {
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
	name, ok := pod.Labels[labelNetSetName]
	if !ok {
		log.Logger.Error("Could not find label for pod", "label", labelNetSetName, "pod", pod.Name)
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
	name, ok := new.Labels[labelNetSetName]
	if !ok {
		log.Logger.Error("Could not find label for pod", "label", labelNetSetName, "pod", new.Name)
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
	name, ok := pod.Labels[labelNetSetName]
	if !ok {
		log.Logger.Error("Could not find label for pod", "label", labelNetSetName, "pod", pod.Name)
		return
	}
	if pod.Status.PodIP != "" {
		r.nsStore.DeleteNet(name, pod.Namespace, fmt.Sprintf("%s/32", pod.Status.PodIP))
		if r.canSync {
			r.nsStore.EnqueueNetSetSync(name, pod.Namespace)
		}
	}
}
