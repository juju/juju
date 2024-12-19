(cloud-kubernetes-aks)=
# The Microsoft AKS cloud and Juju


This document describes details specific to using your existing Microsoft AKS cloud with Juju. 

> See more: [Microsoft AKS](https://azure.microsoft.com/en-us/products/kubernetes-service) 

When using this cloud with Juju, it is important to keep in mind that it is a (1) Kubernetes cloud and (2) not some other cloud.

> See more: {ref}`cloud-differences`

As the differences related to (1) are already documented generically in the rest of the docs, here we record just those that follow from (2).


## Notes on `add-k8s`

Starting with Juju 3.0, because of  the  fact that the `juju` client snap is strictly confined but the AKS cloud CLI snap is not, you must run the `add-k8s` command with the 'raw' client. See note in {ref}`add-a-kubernetes-cloud`.
