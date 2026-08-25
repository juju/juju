As for all Kubernetes clouds, the following {ref}`cloud-specific model configuration keys <model-config-cloud-specific-key>` are supported:

**Storage**

(kubernetes-model-config-operator-storage)=
- **`operator-storage`**: The storage class used to provision operator storage. Type: `string`. Default: `""` (uses cluster default storage class). Immutable.

(kubernetes-model-config-workload-storage)=
- **`workload-storage`**: The preferred storage class used to provision workload storage. Type: `string`. Default: `""` (uses cluster default storage class).

**Pods**

(kubernetes-model-config-enable-service-links)=
- **`enable-service-links`**: Whether Kubernetes service link environment variables are injected into workload pods. Type: `bool`. Default: `true`.

```{ibnote}
See more: {ref}`model-config-cloud-specific-key`
```
