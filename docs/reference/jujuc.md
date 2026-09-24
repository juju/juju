---
myst:
  html_meta:
    description: "jujuc reference: binary providing hook commands for charms to interact with the Juju environment during hook execution."
---

(jujuc)=
# `jujuc`

In Juju, **`jujuc`** is a binary that comes with your `juju` installation which provides {ref}`hook commands <hook-command>` -- a collection of command-line commands that {ref}`charms <charm>` can use during their hook executions to interact with the Juju environment.


```{ggarch}
:file: ../juju.ggarch
:view: K8s deployment topology
:alt: Inside the unit pod: the charm container holds the charm and the unit agent; the workload container holds Pebble. During a hook the charm calls hook commands and the unit agent serves each one via the API server.
:caption: Topology: `jujuc` in the picture: during a hook, every hook command the charm runs is a call to the unit agent, which serves it against the controller's API server. The binary is the charm's only door out.
```
