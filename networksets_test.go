package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/utilitywarehouse/kube-policy-semaphore/log"
)

func TestNetworkSets(t *testing.T) {
	log.InitLogger("test", "debug")
	netsSetStore := NetworkSetStore{
		store:   make(map[string]*NetworkSet),
		cluster: "test",
	}
	assert.Equal(t, "test", netsSetStore.cluster)

	// Add a net to a set
	netsSetStore.AddNet("name", "namespace", "10.0.0.0/24")
	assert.Equal(t, 1, len(netsSetStore.store))
	id := makeNetworkSetID("name", "namespace", "test")
	expectedLables := map[string]string{
		labelManagedBy:       keyManagedBy,
		labelNetSetCluster:   "test",
		labelNetSetName:      "name",
		labelNetSetNamespace: "namespace",
	}
	assert.Equal(t, 1, len(netsSetStore.store[id].nets))
	assert.Equal(t, "10.0.0.0/24", netsSetStore.store[id].nets[0])
	assert.Equal(t, expectedLables, netsSetStore.store[id].labels)

	// Add the same net to the set again - set should remain the same
	netsSetStore.AddNet("name", "namespace", "10.0.0.0/24")
	assert.Equal(t, 1, len(netsSetStore.store))
	assert.Equal(t, 1, len(netsSetStore.store[id].nets))
	assert.Equal(t, "10.0.0.0/24", netsSetStore.store[id].nets[0])
	assert.Equal(t, expectedLables, netsSetStore.store[id].labels)

	// Add a new net to the set again
	netsSetStore.AddNet("name", "namespace", "10.0.0.1/24")
	assert.Equal(t, 1, len(netsSetStore.store))
	assert.Equal(t, 2, len(netsSetStore.store[id].nets))
	assert.Equal(t, "10.0.0.0/24", netsSetStore.store[id].nets[0])
	assert.Equal(t, "10.0.0.1/24", netsSetStore.store[id].nets[1])
	assert.Equal(t, expectedLables, netsSetStore.store[id].labels)

	// Add a different net for a new set
	netsSetStore.AddNet("name2", "namespace2", "10.0.0.1/24")
	assert.Equal(t, 2, len(netsSetStore.store))
	assert.Equal(t, 2, len(netsSetStore.store[id].nets))
	assert.Equal(t, "10.0.0.0/24", netsSetStore.store[id].nets[0])
	assert.Equal(t, "10.0.0.1/24", netsSetStore.store[id].nets[1])
	assert.Equal(t, expectedLables, netsSetStore.store[id].labels)
	id2 := makeNetworkSetID("name2", "namespace2", "test")
	expectedLables2 := map[string]string{
		labelManagedBy:       keyManagedBy,
		labelNetSetCluster:   "test",
		labelNetSetName:      "name2",
		labelNetSetNamespace: "namespace2",
	}
	assert.Equal(t, 1, len(netsSetStore.store[id2].nets))
	assert.Equal(t, "10.0.0.1/24", netsSetStore.store[id2].nets[0])
	assert.Equal(t, expectedLables2, netsSetStore.store[id2].labels)

	// Delete a net from a set
	netsSetStore.DeleteNet("name", "namespace", "10.0.0.1/24")
	assert.Equal(t, 2, len(netsSetStore.store))
	assert.Equal(t, 1, len(netsSetStore.store[id].nets))
	assert.Equal(t, "10.0.0.0/24", netsSetStore.store[id].nets[0])
	assert.Equal(t, expectedLables, netsSetStore.store[id].labels)
	assert.Equal(t, 1, len(netsSetStore.store[id2].nets))
	assert.Equal(t, "10.0.0.1/24", netsSetStore.store[id2].nets[0])
	assert.Equal(t, expectedLables2, netsSetStore.store[id2].labels)

	// Delete the last net from a set - that should delete the set itself
	netsSetStore.DeleteNet("name2", "namespace2", "10.0.0.1/24")
	assert.Equal(t, 1, len(netsSetStore.store))
	assert.Equal(t, 1, len(netsSetStore.store[id].nets))
	assert.Equal(t, "10.0.0.0/24", netsSetStore.store[id].nets[0])
	assert.Equal(t, expectedLables, netsSetStore.store[id].labels)

	// Delete non existing net - should cause no action
	netsSetStore.DeleteNet("name", "namespace", "10.0.0.1/24")
	assert.Equal(t, 1, len(netsSetStore.store))
	assert.Equal(t, 1, len(netsSetStore.store[id].nets))
	assert.Equal(t, "10.0.0.0/24", netsSetStore.store[id].nets[0])
	assert.Equal(t, expectedLables, netsSetStore.store[id].labels)
}
