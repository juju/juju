---
myst:
  html_meta:
    description: "Containeragent reference: binary implementing Juju agent functionality for units in Kubernetes cloud deployments."
---

(containeragent)=
# `containeragent`

In Juju, `containeragent` is a binary that implements {ref}`agent <agent>` functionality for the {ref}`units <unit>` in a Juju deployment on a Kubernetes cloud.

```{ggarch}
:file: ../juju.ggarch
:view: K8s deployment topology
:alt: Unit pod on the right: the charm container holds the container agent and charm code, the workload container holds Pebble and the workload services. Controller pod on the left runs the controller agent with Dqlite in-process and connects down to Charmhub.
:caption: Where `containeragent` runs: one binary per unit pod, containing the unit agent. The charm container holds the agent plus the charm code; the workload container holds Pebble and the workload. The controller pod's agent (`jujud`) runs the controller instead.
```

