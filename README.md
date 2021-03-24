# kube-policy-semaphore

This is an kubernetes operator that wathces pods on a remote cluster based on
a label and an annotation, and creates and manages local calico
GlobalNetworkSets resources that contain the watched pods' ip addresses. As a
result, we can use the produced sets of ips to create local NetworkPolicies for
kubernetes cross cluster pod to pod communication.
