package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	"github.com/utilitywarehouse/semaphore-policy/log"
)

// stubController lets tests control HasSynced without a real informer.
type stubController struct {
	cache.Controller
	synced bool
}

func (c *stubController) HasSynced() bool { return c.synced }

func TestPodWatcherHealthy(t *testing.T) {
	tests := []struct {
		name         string
		synced       bool
		watchHealthy bool
		want         bool
	}{
		{"synced and watching", true, true, true},
		{"synced but watch failing", true, false, false},
		{"watching but not synced", false, true, false},
		{"neither", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pw := &PodWatcher{controller: &stubController{synced: tt.synced}}
			pw.watchHealthy.Store(tt.watchHealthy)
			assert.Equal(t, tt.want, pw.Healthy())
		})
	}
}

// Reproduces the production setup: client-go (>= v0.35) populates the cache over
// a streaming watch (WatchListClient), so the list handler is never called and
// health must not depend on it.
func TestPodWatcherHealthyAfterSync(t *testing.T) {
	log.InitLogger("test", "debug")

	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:            "p",
		Namespace:       "ns",
		Labels:          map[string]string{"netset": "x"},
		ResourceVersion: "1",
	}}
	client := fake.NewClientset()
	client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		t.Error("list must not be called when the cache is populated by a streaming watch")
		return true, &v1.PodList{}, nil
	})
	client.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		w := watch.NewFakeWithOptions(watch.FakeOptions{ChannelSize: 2})
		w.Add(pod)
		w.Action(watch.Bookmark, &v1.Pod{ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "1",
			Annotations:     map[string]string{metav1.InitialEventsAnnotationKey: "true"},
		}})
		return true, w, nil
	})

	pw := NewPodWatcher(client, 0, func(watch.EventType, *v1.Pod, *v1.Pod) {}, "netset")
	pw.Init()
	assert.False(t, pw.Healthy(), "must not be healthy before the watcher runs")

	go pw.Run()
	defer pw.Stop()

	assert.Eventually(t, pw.Healthy, 5*time.Second, 10*time.Millisecond)
}
