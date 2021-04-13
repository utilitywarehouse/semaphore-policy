// Package metrics contains global structures for capturing
// semaphore-policy metrics. The following metrics are implemented:
//
//   - kube_policy_semaphore_calico_client_request{"type", "success"}
//   - kube_policy_semaphore_pod_watcher_failures{"type"}
//   - kube_policy_semaphore_sync_queue_full_failures{"globalnetworkset"}
//   - kube_policy_semaphore_sync_requeue{"globalnetworkset"}
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	calicoClientRequest = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_policy_semaphore_calico_client_request",
			Help: "Counts calico client requests.",
		},
		[]string{"type", "success"},
	)
	podWatcherFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_policy_semaphore_pod_watcher_failures",
			Help: "Number of failed pod watcher actions (watch|list).",
		},
		[]string{"type"},
	)
	syncQueueFullFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_policy_semaphore_sync_queue_full_failures",
			Help: "Number of times a sync task was not added to the sync queue in time because the queue was full.",
		},
		[]string{"globalnetworkset"},
	)
	syncRequeue = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_policy_semaphore_sync_requeue",
			Help: "Number of attempts to requeue a sync.",
		},
		[]string{"globalnetworkset"},
	)
)

func init() {
	prometheus.MustRegister(calicoClientRequest)
	prometheus.MustRegister(podWatcherFailures)
	prometheus.MustRegister(syncQueueFullFailures)
	prometheus.MustRegister(syncRequeue)
}

func IncCalicoClientRequest(t string, err error) {
	s := "1"
	if err != nil {
		s = "0"
	}
	calicoClientRequest.With(prometheus.Labels{
		"type":    t,
		"success": s,
	}).Inc()
}

func IncPodWatcherFailures(t string) {
	podWatcherFailures.With(prometheus.Labels{
		"type": t,
	}).Inc()
}

func IncSyncQueueFullFailures(globalnetworkset string) {
	syncQueueFullFailures.With(prometheus.Labels{
		"globalnetworkset": globalnetworkset,
	}).Inc()
}

func IncSyncRequeue(globalnetworkset string) {
	syncRequeue.With(prometheus.Labels{
		"globalnetworkset": globalnetworkset,
	}).Inc()
}
