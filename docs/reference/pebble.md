(pebble)=
# Pebble

**Pebble** is a lightweight, API-driven process supervisor. When Juju provisions resources on a Kubernetes cloud, Pebble is automatically injected into each container, where it acts like an `init` system, and  charm interacts with its workload through the workload container's Pebble.


> See more: [Pebble documentation](https://canonical-pebble.readthedocs-hosted.com/en/latest/), {ref}`Debugging tips <debug-a-k8s-charm>`


## Pebble notices

In Pebble, a **notice** is an aggregated event to record when custom events happen in the workload container or in Pebble itself.

> See more: [Pebble | Notices](https://canonical-pebble.readthedocs-hosted.com/en/latest/reference/notices/#)

Pebble notices are supported in Juju starting with version 3.4. Juju polls each workload container's Pebble server for new notices, and fires an event to the charm when a notice first occurs as well as each time it repeats.

Each notice has a *type* and *key*, the combination of which uniquely identifies it. A notice's count of occurrences is incremented every time a notice with that type and key combination occurs.

Currently, the only notice type is "custom". These are custom notices recorded by a user of Pebble; in future, other notice types may be recorded by Pebble itself. When a custom notice occurs, Juju fires a [`PebbleCustomNoticeEvent`](https://ops.readthedocs.io/en/latest/#ops.PebbleCustomNoticeEvent) event whose [`workload`](https://ops.readthedocs.io/en/latest/#ops.WorkloadEvent.workload) attribute is set to the relevant container.

Custom notices allow the workload to wake up the charm when something interesting happens with the workload, for example, when a PostgreSQL backup process finishes, or some kind of alert occurs.
