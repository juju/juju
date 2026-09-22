---
myst:
  html_meta:
    description: "jujud reference: executable binary implementing Juju agent functionality for machines and controllers in cloud deployments."
---

(jujud)=
# `jujud`

In Juju, `jujud` is the executable binary that is produced at each release and which implements {ref}`agent <agent>` functionality for all of the entities in a Juju deployment on a machine cloud (model, machine, unit, controller) and also some of the entities in a Juju deployment on a Kubernetes cloud (model, controller).

```{ggarch}
:file: ../juju.ggarch
:view: Worker tree (machine cloud)
:alt: A machine cloud node at the top, the machine agent below it with its workers (machine lock, updater, logging, http server), and the unit agent beneath with its own workers including the uniter, which runs the charm. Arrows name what each worker does.
```
*On a machine cloud, `jujud` runs every agent role. The machine agent's
workers manage the machine itself; the unit agent runs the charm; a
separate controller agent (not shown here) carries the model and
controller roles with the embedded Dqlite database.*


