# semaphore-policy

This is an kubernetes operator that watches pods on a remote cluster based on
a label and creates and manages local calico GlobalNetworkSets resources that
contain the watched pods' ip addresses. As a result, we can use the produced
sets of ips to create local NetworkPolicies for kubernetes cross cluster pod to
pod communication.

# Usage

## Flags

```
Usage of ./semaphore-policy:
  -local-kube-config string
        Path of the local kube cluster config file, if not provided the app will try to get in cluster config
  -log-level string
        Log level (default "info")
  -pod-resync-period duration
        Pod watcher cache resync period. Disabled by default
  -remote-api-url string
        Remote Kubernetes API server URL
  -remote-ca-url string
        Remote Kubernetes CA certificate URL
  -remote-sa-token-path string
        Remote Kubernetes cluster token path
  -target-cluster-name string
        (required) The name of the cluster from which pods are synced as networksets. It will also be used as a prefix used when creating network sets.
  -target-kube-config string
        (Required) Path of the target cluster kube config file to watch pods
```

## Operator

  The policy operator will watch the target cluster pods which are labelled
with: `policy.semaphore.uw.io/name`. For these pods it will extract a name from
the label and will use it along with the namespace of the pod and the cluster it
resides to create a GlobalNetworkSet resource (or amend an existing one) on the
local cluster. Using namespace and cluster name will help avoiding conflicts
with workloads from different locations that want to use the same value for
`policy.semaphore.uw.io/name`.

  For example annotating a pod with `policy.semaphore.uw.io/name=my-app` under a
namespace called `my-ns` in a cluster called `my-cluster` will tell the operator
to add the pod's ip to a network set named my-cluster-my-ns-my-app. In order to
make it easier to select that set in network policies, the following labels will
be added:
```
managed-by=calico-global-network-sync-operator
policy.semaphore.uw.io/name=my-app
policy.semaphore.uw.io/namespace=my-ns
policy.semaphore.uw.io/cluster=my-cluster
```

  Thus, one can use the above labels to target the created GlocalNetworkSet
inside a calico network policy on the local cluster and allow traffic from the
set pods.

### Example Generated GlobalNetworkSets

Example of a generated global network set from the operator:
```
Name:         my-cluster-my-ns-my-app
Namespace:
Labels:       managed-by=calico-global-network-sync-operator
              policy.semaphore.uw.io/name=my-app
              policy.semaphore.uw.io/namespace=my-ns
              policy.semaphore.uw.io/cluster=my-cluster
API Version:  crd.projectcalico.org/v1
Kind:         GlobalNetworkSet
Spec:
  Nets:
    10.4.1.38/32

```

### Example Calico Network Policy

The following network policy will allow traffic from the above set:
```
apiVersion: crd.projectcalico.org/v1
kind: NetworkPolicy
metadata:
  name: allow-from-remote-cluster
  namespace: local-ns
spec:
  selector: app == 'my-app'
  types:
  - Ingress
  ingress:
  - action: Allow
    protocol: TCP
    source:
      selector: policy.semaphore.uw.io/name == 'my-app' && policy.semaphore.uw.io/namespace == 'my-ns' && policy.semaphore.uw.io/cluster == 'my-cluster'
      namespaceSelector: global()
```

* `namespaceSelector: global()` is needed so that the namespaced network policy
is able to bind to GlobalNetworkSets.

# Deploy

In order to deploy semaphore-policy, first we need to deploy a service
account to the remote target cluster and grant it the required permissions to
be able to watch pods. For that one could use our kustomize [base](./deploy/kustomize/remote/)
directly.
Then a local cluster deployment of the operator is required. An example
deploying the operator under `kube-system` namespace can be found [here](./deploy/example).
