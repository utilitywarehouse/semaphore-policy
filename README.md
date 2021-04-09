# kube-policy-semaphore

This is an kubernetes operator that watches pods on a remote cluster based on
a label and an annotation, and creates and manages local calico
GlobalNetworkSets resources that contain the watched pods' ip addresses. As a
result, we can use the produced sets of ips to create local NetworkPolicies for
kubernetes cross cluster pod to pod communication.

# Usage

## Flags

```
Usage of ./kube-policy-semaphore:
  -full-store-resync-period duration
        Frequency to perform a full network set store resync from cache to calico GlocalNetworkPolicies (default 1h0m0s)
  -label-selector string
        Label of pods to watch and create/update network sets. (default "uw.systems/networksets=true")
  -local-kube-config string
        Path of the local kube cluster config file, if not provided the app will try to get in cluster config
  -log-level string
        Log level (default "info")
  -networkset-name-annotation string
        Pod annotation with the name of the set the pod belong to (default "uw.systems/networkset-name")
  -pod-resync-period duration
        Pod watcher cache resync period (default 1h0m0s)
  -remote-api-url string
        Remote Kubernetes API server URL
  -remote-ca-url string
        Remote Kubernetes CA certificate URL
  -remote-sa-token-path string
        Remote Kubernetes cluster token path
  -target-cluster-name string
        (required) The name of the cluster from which pods are synced as networksets.It will also be used as a prefix used when creating network sets.
  -target-kube-config string
        (Required) Path of the target cluster kube config file to add wg peers from
```

## Operator

  Deploying with the default values for label selector and networkset name
annotation (see usage above), kube-policy-semaphore will watch the target
cluster pods which are labelled with: `uw.systems/networksets=true`. For these
pods it will extract a network set name from the respective annotation and
create a GlobalNetworkSet resource (or amend an existing one) on the local
cluster.

  For example annotating a pod with `uw.systems/networkset-name=my-set` will
tell the operator to add the pod ip to a network set named my-set. In order to
avoid conflicting with other namespaces' network sets, the operator will try to
create a GlobalNetworkSet calico resource named:
`<remote-cluster>-<remote-pod-namespace>-my-set`. Additionally the
GlobalNetworkSet will be labelled with the following:
```
managed-by=calico-global-network-sync-operator
name=my-set
namespace=<remote-pod-namespace>
remote-cluster-name=<remote-cluster>
```

  Thus, one can use the above labels to target the created GlocalNetworkSet
inside a calico network policy on the local cluster and allow traffic from the
set pods.

### Example Generated GlobalNetworkSets

Example of a generated global network set from the operator:
```
Name:         <remote-cluster>-<remote-pod-namespace>-my-set
Namespace:
Labels:       managed-by=calico-global-network-sync-operator
              name=my-set
              namespace=<remote-pod-namespace>
              remote-cluster-name=<remote-cluster>
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
      selector: name == 'my-set' && namespace == '<remote-pod-namespace>' && remote-cluster-prefix == '<remote-cluster>'
      namespaceSelector: global()
```

* `namespaceSelector: global()` is needed so that the namespaced network policy
is able to bind to GlobalNetworkSets.

# Deploy

In order to deploy kube-policy-semaphore, first we need to deploy a service
account to the remote target cluster and grant it the required permissions to
be able to watch pods. For that one could use our kustomize [base](./deploy/kustomize/remote/)
directly.
Then a local cluster deployment of the operator is required. An example
deploying the operator under `kube-system` namespace can be found [here](./deploy/example).
