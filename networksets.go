package main

import (
	"fmt"

	calicoClient "github.com/projectcalico/libcalico-go/lib/clientv3"

	"github.com/utilitywarehouse/kube-policy-semaphore/calico"
	"github.com/utilitywarehouse/kube-policy-semaphore/log"
)

type NetworkSet struct {
	labels map[string]string
	nets   []string
}

type NetworkSetStore struct {
	store   map[string]*NetworkSet
	cluster string // the name of the cluster that contains targets of this set
}

func newNetworkSetStore(cluster string) NetworkSetStore {
	return NetworkSetStore{
		store:   make(map[string]*NetworkSet),
		cluster: cluster,
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

func (nss *NetworkSetStore) SyncToCalico(client calicoClient.Interface, name, namespace string) error {
	id := makeNetworkSetID(name, namespace, nss.cluster)
	netset, ok := nss.store[id]
	if !ok {
		log.Logger.Info(
			"Could not find network set in store, will try deleting from calico",
			"resource", id)
		return calico.DeleteGlobalNetworkSet(client, id)
	}
	log.Logger.Info("Updating calico object", "resource", id, "nets", netset.nets)
	return calico.CreateOrUpdateGlobalNetworkSet(
		client,
		id,
		netset.labels,
		netset.nets,
	)
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
