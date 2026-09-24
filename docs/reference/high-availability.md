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

In the context of a cloud deployment in general, **high availability (HA)** is the concept of making software resilient to failures by means of running multiple replicas with shared and synchronised software context -- something usually achieved through coordinated {ref}`scaling (out and up) <scaling>`. In Juju, it is supported for controllers on machine clouds and for regular applications on both machine and Kubernetes clouds

Note: Controller high availability is currently only supported on machine clouds -- it is not supported (along with backup and restore) for Kubernetes controllers. See {ref}`manage-controllers`.


```{ggarch}
:file: ../juju.ggarch
:view: HA controller: Dqlite replicaset
:alt: Three machine nodes side by side, each running a controller agent with an embedded Dqlite database, connected by replicate-arrows between the databases.
:caption: Topology: Controller high availability (machine clouds). Juju controllers can be made highly-available by enabling more than one machine to each run a separate controller unit with a separate controller agent instance, where each machine effectively becomes an instance of the controller. This set of Juju agents collectively use a Dqlite database replicaset to achieve data synchronisation amongst them.
```
