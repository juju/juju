(cloud-kubernetes-gke)=
# The Google GKE cloud and Juju

<!--To see the older HTG-style doc, see version 21. Note that it may be out-of-date. -->


This document describes details specific to using your existing Google GKE cloud with Juju. 

> See more: [Google GKE](https://cloud.google.com/kubernetes-engine/docs) 

When using this cloud with Juju, it is important to keep in mind that it is a (1) Kubernetes cloud and (2) not some other cloud.

> See more: {ref}`cloud-differences`

As the differences related to (1) are already documented generically in the rest of the docs, here we record just those that follow from (2).

## Notes on `add-k8s`

Starting with Juju 3.0, because of the  fact that the `juju` client snap is strictly confined but the GKE cloud CLI snap is not, you must run the `add-k8s` command with the 'raw' client. See note in {ref}`add-a-kubernetes-cloud`.
