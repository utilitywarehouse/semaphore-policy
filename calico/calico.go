package calico

import (
	"context"

	"github.com/projectcalico/libcalico-go/lib/apiconfig"
	v3 "github.com/projectcalico/libcalico-go/lib/apis/v3"
	client "github.com/projectcalico/libcalico-go/lib/clientv3"
	calicoOptions "github.com/projectcalico/libcalico-go/lib/options"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	if err != nil {
		// If Get errors try to create a new globalnetworkset
		gns = &v3.GlobalNetworkSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: labels,
			},
			Spec: v3.GlobalNetworkSetSpec{Nets: nets},
		}
		_, err := client.GlobalNetworkSets().Create(ctx, gns, calicoOptions.SetOptions{})
		return err
	}
	// Else update the existing one
	gns.Labels = labels
	gns.Spec.Nets = nets
	_, err = client.GlobalNetworkSets().Update(ctx, gns, calicoOptions.SetOptions{})
	return err
}

// DeleteGlobalNetworkSet will delete a GlobalNetworkSet if exists
func DeleteGlobalNetworkSet(client client.Interface, name string) error {
	ctx := context.Background()
	_, err := client.GlobalNetworkSets().Get(ctx, name, calicoOptions.GetOptions{})
	if err != nil {
		return err
	}
	_, err = client.GlobalNetworkSets().Delete(ctx, name, calicoOptions.DeleteOptions{})
	return err
}

// GetGlobalNetworkSet returns a list of sets that contain the passed labels
func GetGlobalNetworkSet(client client.Interface, labels map[string]string) ([]v3.GlobalNetworkSet, error) {
	ctx := context.Background()
	// calico GlobalNetworkSets List cannot use labels as selector, so we
	// will have to fetch them all and make the selection manually
	netsetlist, err := client.GlobalNetworkSets().List(ctx, calicoOptions.ListOptions{})
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
			if !ok {
				match = false
				break
			}
			if v != value {
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
