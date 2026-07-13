```{ibnote}
See also: {ref}`unit`, {ref}`Juju | Manage units <manage-units>`
```

As for all Kubernetes clouds, the following constraints apply to pod resources and placement.

(kubernetes-constraints)=
The following {ref}`constraints <constraint>` apply to pod resources and placement behavior:

- {ref}`constraint-cpu-power`. CPU resource request/limit for pods.
- {ref}`constraint-mem`. Memory resource request/limit for pods.
- {ref}`constraint-tags`. Used for pod affinity, anti-affinity, and node affinity rules.

```{ibnote}
Constraints like `arch`, `cores`, `instance-type`, `root-disk`, `zones`, and others are not supported on Kubernetes clouds. Kubernetes manages node resources and pod scheduling.
```

(kubernetes-placement-directives)=
Placement directives are not supported on Kubernetes clouds. Pod placement is controlled by Kubernetes scheduling, node selectors, and affinity rules (configured via constraints).
