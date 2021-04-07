package main

import (
	"fmt"
	"time"

	calicoClient "github.com/projectcalico/libcalico-go/lib/clientv3"

	"github.com/utilitywarehouse/kube-policy-semaphore/calico"
	"github.com/utilitywarehouse/kube-policy-semaphore/log"
)

type NetworkSet struct {
	labels map[string]string
	nets   []string
}

type SyncObject struct {
	id string
}

type NetworkSetStore struct {
	client        calicoClient.Interface
	syncQueue     chan SyncObject
	fullSyncQueue chan struct{}
	stop          chan struct{}
	store         map[string]*NetworkSet
	cluster       string // the name of the cluster that contains targets of this set
}

func newNetworkSetStore(cluster string, client calicoClient.Interface) NetworkSetStore {
	return NetworkSetStore{
		client:        client,
		store:         make(map[string]*NetworkSet),
		cluster:       cluster,
		syncQueue:     make(chan SyncObject),
		fullSyncQueue: make(chan struct{}),
		stop:          make(chan struct{}),
	}
}

func (nss *NetworkSetStore) addNetworkSet(id, name, namespace, net string) *NetworkSet {
	labels := map[string]string{
		labelManagedBy:         keyManagedBy,
		labelRemoteClusterName: nss.cluster,
		labelNetSetName:        name,
		labelNetSetNamespace:   namespace,
	}
	ns := &NetworkSet{
		labels: labels,
		nets:   []string{net},
	}
	nss.store[id] = ns
	return ns
}

func (nss *NetworkSetStore) deleteNetworkSet(id string) {
	if _, ok := nss.store[id]; !ok {
		return
	}
	delete(nss.store, id)
}

func (nss *NetworkSetStore) AddNet(name, namespace, net string) *NetworkSet {
	id := makeNetworkSetID(name, namespace, nss.cluster)
	netset, ok := nss.store[id]
	if !ok {
		return nss.addNetworkSet(id, name, namespace, net)
	}
	if _, found := inSlice(netset.nets, net); !found {
		netset.nets = append(netset.nets, net)
	}
	nss.store[id] = netset
	log.Logger.Debug("Added new net to set", "resource", id, "net", net, "set", netset.nets)
	return netset
}

func (nss *NetworkSetStore) DeleteNet(name, namespace, net string) *NetworkSet {
	id := makeNetworkSetID(name, namespace, nss.cluster)
	netset, ok := nss.store[id]
	if !ok {
		return nil
	}
	if i, found := inSlice(netset.nets, net); found {
		netset.nets = removeFromSlice(netset.nets, i)
	}
	nss.store[id] = netset
	log.Logger.Debug("Deleted net from set", "resource", id, "net", net, "set", netset.nets)
	if len(netset.nets) == 0 {
		log.Logger.Debug("Deleting empty network set", "resource name", id)
		nss.deleteNetworkSet(id)
		return nil
	}
	return netset
}

func (nss *NetworkSetStore) syncToCalico(id string) error {
	netset, ok := nss.store[id]
	if !ok {
		log.Logger.Info(
			"Could not find network set in store, will try deleting from calico",
			"resource", id)
		return calico.DeleteGlobalNetworkSet(nss.client, id)
	}
	log.Logger.Info("Updating calico object", "resource", id, "nets", netset.nets)
	return calico.CreateOrUpdateGlobalNetworkSet(
		nss.client,
		id,
		netset.labels,
		netset.nets,
	)
}

// RunSyncLoop is the main loop to handle sync signals for network sets and
// full store syncs.
func (nss *NetworkSetStore) RunSyncLoop() {
	for {
		select {
		case o := <-nss.syncQueue:
			if err := nss.syncToCalico(o.id); err != nil {
				log.Logger.Error("failed to sync netset to calico GlobalNetworkSets", "id", o.id)
				nss.requeue(o.id)
			}
		case <-nss.fullSyncQueue:
			log.Logger.Debug("staring a new full sync loop")
			currentNetSets, err := calico.GlobalNetworkSetList(nss.client, map[string]string{
				labelManagedBy:         keyManagedBy,
				labelRemoteClusterName: nss.cluster,
			})
			if err != nil {
				log.Logger.Error("failed get the list of existing network sets, potential stale set left behind!", "cluster", nss.cluster)
			}
			for _, n := range currentNetSets {
				// if network set is not in the store, trigger a sync that will delete it from kube resources as well.
				// Otherwise it will be updated bellow.
				if _, ok := nss.store[n.Name]; !ok {
					if err := nss.syncToCalico(n.Name); err != nil {
						log.Logger.Error("failed to sync netset to calico GlobalNetworkSets", "id", n.Name)
						nss.requeue(n.Name)
					}
				}
			}
			for id, _ := range nss.store {
				if err := nss.syncToCalico(id); err != nil {
					log.Logger.Error("failed to sync netset to calico GlobalNetworkSets", "id", id)
					nss.requeue(id)
				}
			}
		case <-nss.stop:
			log.Logger.Debug("Stopping network set store loop")
			return
		}
	}
}

func (nss *NetworkSetStore) requeue(id string) {
	log.Logger.Debug("Requeueing sync task", "id", id)
	go func() {
		time.Sleep(1)
		nss.enqueue(id)
	}()
}

// write to the sync queue or yield an error after 5 seconds and retry.
func (nss *NetworkSetStore) enqueue(id string) {
	select {
	case nss.syncQueue <- SyncObject{id: id}:
		log.Logger.Debug("Sync task queued", "id", id)
	case <-time.After(5 * time.Second):
		log.Logger.Error("Timed out trying to queue a sync action for netset, run queue is full", "id", id)
		nss.requeue(id)
	}
}

// EnqueueSync calculates the network set store id and adds to the sync queue
func (nss *NetworkSetStore) EnqueueNetSetSync(name, namespace string) {
	id := makeNetworkSetID(name, namespace, nss.cluster)
	nss.enqueue(id)
}

// makeNetworkSetID returns the name of the respective calico GlobalNetworkSet
func makeNetworkSetID(name, namespace, cluster string) string {
	return fmt.Sprintf("%s-%s-%s", cluster, namespace, name)
}

func inSlice(slice []string, val string) (int, bool) {
	for i, item := range slice {
		if item == val {
			return i, true
		}
	}
	return -1, false
}

func removeFromSlice(slice []string, i int) []string {
	slice[len(slice)-1], slice[i] = slice[i], slice[len(slice)-1]
	return slice[:len(slice)-1]
}
