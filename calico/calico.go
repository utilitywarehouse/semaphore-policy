package calico

import (
	"context"

	"github.com/projectcalico/libcalico-go/lib/apiconfig"
	v3 "github.com/projectcalico/libcalico-go/lib/apis/v3"
	client "github.com/projectcalico/libcalico-go/lib/clientv3"
	"github.com/projectcalico/libcalico-go/lib/errors"
	calicoOptions "github.com/projectcalico/libcalico-go/lib/options"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/utilitywarehouse/semaphore-policy/metrics"
)

// NewClient return a calico client
func NewClient(kubeconfig string) (client.Interface, error) {

	if kubeconfig != "" {
		return newForConfig(kubeconfig)
	} else {
		return client.NewFromEnv()
	}
}

func newForConfig(kubeconfig string) (client.Interface, error) {
	return client.New(apiconfig.CalicoAPIConfig{
		Spec: apiconfig.CalicoAPIConfigSpec{
			DatastoreType: apiconfig.Kubernetes,
			KubeConfig: apiconfig.KubeConfig{
				Kubeconfig:               kubeconfig,
				K8sInsecureSkipTLSVerify: false,
			},
		},
	})
}

// CreateOrUpdateGlobalNetworkSet will try to get a globalNetworkSet and update if exists, otherwise create a new one
func CreateOrUpdateGlobalNetworkSet(client client.Interface, name string, labels map[string]string, nets []string) error {
	ctx := context.Background()
	gns, err := client.GlobalNetworkSets().Get(ctx, name, calicoOptions.GetOptions{})
	if _, ok := err.(errors.ErrorResourceDoesNotExist); ok {
		// Try creating if the resource does not exist
		gns = &v3.GlobalNetworkSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: labels,
			},
			Spec: v3.GlobalNetworkSetSpec{Nets: nets},
		}
		_, err := client.GlobalNetworkSets().Create(ctx, gns, calicoOptions.SetOptions{})
		metrics.IncCalicoClientRequest("create", err)
		return err
	}
	if err != nil {
		metrics.IncCalicoClientRequest("get", err)
		return err
	}
	// Else update the existing one
	gns.Labels = labels
	gns.Spec.Nets = nets
	_, err = client.GlobalNetworkSets().Update(ctx, gns, calicoOptions.SetOptions{})
	metrics.IncCalicoClientRequest("update", err)
	return err
}

// DeleteGlobalNetworkSet will delete a GlobalNetworkSet if exists
func DeleteGlobalNetworkSet(client client.Interface, name string) error {
	ctx := context.Background()
	_, err := client.GlobalNetworkSets().Get(ctx, name, calicoOptions.GetOptions{})
	metrics.IncCalicoClientRequest("get", err)
	if err != nil {
		return err
	}
	_, err = client.GlobalNetworkSets().Delete(ctx, name, calicoOptions.DeleteOptions{})
	metrics.IncCalicoClientRequest("delete", err)
	return err
}

// GlobalNetworkSetList returns a list of sets that can match all the passed
// labels (AND matching)
func GlobalNetworkSetList(client client.Interface, labels map[string]string) ([]v3.GlobalNetworkSet, error) {
	ctx := context.Background()
	// calico GlobalNetworkSets List cannot use labels as selector, so we
	// will have to fetch them all and make the selection manually
	netsetlist, err := client.GlobalNetworkSets().List(ctx, calicoOptions.ListOptions{})
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
