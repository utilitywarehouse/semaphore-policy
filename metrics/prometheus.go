package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	calicoClientRequest = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "semaphore_policy_calico_client_request_total",
			Help: "Counts calico client requests.",
		},
		[]string{"type", "success"},
	)
	podWatcherFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "semaphore_policy_pod_watcher_failures_total",
			Help: "Number of failed pod watcher actions (watch|list).",
		},
		[]string{"type"},
	)
	syncQueueFullFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "semaphore_policy_sync_queue_full_failures_total",
			Help: "Number of times a sync task was not added to the sync queue in time because the queue was full.",
		},
	)
	syncRequeue = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "semaphore_policy_sync_requeue_total",
			Help: "Number of attempts to requeue a sync.",
		},
	)
)

func init() {
	// Retrieving a Counter from a CounterVec will initialize it with a 0 value if it
	// doesn't already have a value. This ensures that all possible counters
	// start with a 0 value.
	for _, t := range []string{"get", "list", "create", "update", "patch", "watch", "delete"} {
		podWatcherFailures.With(prometheus.Labels{"type": t})
		for _, s := range []string{"0", "1"} {
			calicoClientRequest.With(prometheus.Labels{"type": t, "success": s})
		}
	}

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

func IncSyncQueueFullFailures() {
	syncQueueFullFailures.Inc()
}

func IncSyncRequeue() {
	syncRequeue.Inc()
}
