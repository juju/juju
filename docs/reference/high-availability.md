---
myst:
  html_meta:
    description: "High availability in Juju: multi-replica controllers, database replication, and resilient application deployment on machines and Kubernetes."
---

(high-availability)=
# High availability (HA)

```{ibnote}
See also:

- {ref}`make-a-controller-highly-available`
- {ref}`make-an-application-highly-available`
```

In the context of a cloud deployment in general, **high availability (HA)** is the concept of making software resilient to failures by means of running multiple replicas with shared and synchronised software context -- something usually achieved through coordinated {ref}`scaling (out and up) <scaling>`. In Juju, it is supported for controllers on machine clouds and for regular applications on both machine and Kubernetes clouds.

On a machine cloud, each controller instance runs on a separate machine. The instances share state through a Dqlite cluster. See {ref}`controller-high-availability` for node roles, cluster membership, and adding or removing capacity, and {ref}`controller-api-addresses` for address discovery. For application resilience, see {ref}`how to make an application highly available <make-an-application-highly-available>`.
