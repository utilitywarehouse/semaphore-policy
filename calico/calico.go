package calico

import (
	"context"
	"fmt"

	v3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	"github.com/projectcalico/api/pkg/client/clientset_generated/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/utilitywarehouse/semaphore-policy/kube"
	"github.com/utilitywarehouse/semaphore-policy/log"
	"github.com/utilitywarehouse/semaphore-policy/metrics"
)

// ClientFromConfig returns a calico client (clientset) from the kubeconfig
// path or from the in-cluster service account environment.
func ClientFromConfig(path string) (*clientset.Clientset, error) {
	conf, err := kube.GetClientConfig(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get Calico client config: %v", err)
	}
	return clientset.NewForConfig(conf)
}

// CreateOrUpdateGlobalNetworkSet will try to get a globalNetworkSet and update if exists, otherwise create a new one
func CreateOrUpdateGlobalNetworkSet(client *clientset.Clientset, name string, labels map[string]string, nets []string) error {
	ctx := context.Background()
	gns, err := client.ProjectcalicoV3().GlobalNetworkSets().Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		log.Logger.Debug("GlobalNetworkSet not found error returned from apu", "set", name)
		metrics.IncCalicoClientRequest("get", nil) // Don't record an error since ErrorResourceDoesNotExist is expected at this point
		// Try creating if the resource does not exist
		gns = &v3.GlobalNetworkSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: labels,
			},
			Spec: v3.GlobalNetworkSetSpec{Nets: nets},
		}
		_, err := client.ProjectcalicoV3().GlobalNetworkSets().Create(ctx, gns, metav1.CreateOptions{})
		metrics.IncCalicoClientRequest("create", err)
		return err
	}
	metrics.IncCalicoClientRequest("get", err)
	if err != nil {
		return err
	}
	// Else update the existing one
	gns.Labels = labels
	gns.Spec.Nets = nets
	_, err = client.ProjectcalicoV3().GlobalNetworkSets().Update(ctx, gns, metav1.UpdateOptions{})
	metrics.IncCalicoClientRequest("update", err)
	return err
}

// DeleteGlobalNetworkSet will try to delete a GlobalNetworkSet
func DeleteGlobalNetworkSet(client *clientset.Clientset, name string) error {
	ctx := context.Background()
	err := client.ProjectcalicoV3().GlobalNetworkSets().Delete(ctx, name, metav1.DeleteOptions{})
	metrics.IncCalicoClientRequest("delete", err)
	return err
}

// GlobalNetworkSetList returns a list of sets that can match all the passed
// labels (AND matching)
func GlobalNetworkSetList(client *clientset.Clientset, labels map[string]string) ([]v3.GlobalNetworkSet, error) {
	ctx := context.Background()
	// calico GlobalNetworkSets List cannot use labels as selector, so we
	// will have to fetch them all and make the selection manually
	netsetlist, err := client.ProjectcalicoV3().GlobalNetworkSets().List(ctx, metav1.ListOptions{})
	metrics.IncCalicoClientRequest("list", err)
	if err != nil {
		return []v3.GlobalNetworkSet{}, err
	}
	if len(labels) == 0 {
		return netsetlist.Items, nil
	}
	var netsets []v3.GlobalNetworkSet
	for _, set := range netsetlist.Items {
		match := true
		for key, value := range labels {
			v, ok := set.Labels[key]
			if !ok || v != value {
				match = false
				break
			}
		}
		if match {
			netsets = append(netsets, set)
		}
	}
	return netsets, nil
}
