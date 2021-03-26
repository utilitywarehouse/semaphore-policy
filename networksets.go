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
	store  map[string]*NetworkSet
	prefix string
}

func newNetworkSetStore(prefix string) NetworkSetStore {
	return NetworkSetStore{
		store:  make(map[string]*NetworkSet),
		prefix: prefix,
	}
}

func (nss *NetworkSetStore) addNetworkSet(name, nsName, nsNamespace, net string) *NetworkSet {
	labels := map[string]string{
		labelManagedBy:           keyManagedBy,
		labelRemoteClusterPrefix: nss.prefix,
		"name":                   nsName,
		"namespace":              nsNamespace,
	}
	ns := &NetworkSet{
		labels: labels,
		nets:   []string{net},
	}
	nss.store[name] = ns
	return ns
}

func (nss *NetworkSetStore) deleteNetworkSet(name string) {
	if _, ok := nss.store[name]; !ok {
		return
	}
	delete(nss.store, name)
}

func (nss *NetworkSetStore) AddNet(nsName, nsNamespace, net string) *NetworkSet {
	name := makeNetworkSetName(nsName, nsNamespace, nss.prefix)
	netset, ok := nss.store[name]
	if !ok {
		return nss.addNetworkSet(name, nsName, nsNamespace, net)
	}
	if _, found := inSlice(netset.nets, net); !found {
		netset.nets = append(netset.nets, net)
	}
	nss.store[name] = netset
	log.Logger.Debug("Added new net to set", "name", name, "net", net, "set", netset.nets)
	return netset
}

func (nss *NetworkSetStore) DeleteNet(nsName, nsNamespace, net string) *NetworkSet {
	name := makeNetworkSetName(nsName, nsNamespace, nss.prefix)
	netset, ok := nss.store[name]
	if !ok {
		return nil
	}
	if i, found := inSlice(netset.nets, net); found {
		netset.nets = removeFromSlice(netset.nets, i)
	}
	nss.store[name] = netset
	log.Logger.Debug("Deleted net from set", "name", name, "net", net, "set", netset.nets)
	if len(netset.nets) == 0 {
		nss.deleteNetworkSet(nsName)
		return nil
	}
	return netset
}

func (nss *NetworkSetStore) SyncToCalico(client calicoClient.Interface, nsName, nsNamespace string) error {
	name := makeNetworkSetName(nsName, nsNamespace, nss.prefix)
	netset, ok := nss.store[name]
	if !ok {
		log.Logger.Info(
			"Could not find network set in store, will try deleting from calico",
			"name", name)
		return calico.DeleteGlobalNetworkSet(client, name)
	}
	log.Logger.Info("Updating calico object", "name", name, "nets", netset.nets)
	return calico.CreateOrUpdateGlobalNetworkSet(
		client,
		name,
		netset.labels,
		netset.nets,
	)
}

func makeNetworkSetName(name, namespace, prefix string) string {
	return fmt.Sprintf("%s-%s-%s", prefix, namespace, name)
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
