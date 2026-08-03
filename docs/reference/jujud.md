---
myst:
  html_meta:
    description: "jujud reference: executable binary implementing Juju agent functionality for machines and controllers in cloud deployments."
---

(jujud)=
# `jujud`

In Juju, `jujud` was historically the executable binary that implemented {ref}`agent <agent>` functionality for all of the entities in a Juju deployment on a machine cloud (model, machine, unit, controller) and also some of the entities in a Juju deployment on a Kubernetes cloud (model, controller).

As of the standalone controller work (JUJU-9694), the `jujud` controller binary is no longer built. The active agent binary is `jujuagentd`, which handles machine, model, unit, and controller agent responsibilities.

