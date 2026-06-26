(kubernetes-bootstrap-service-type)=
As for all Kubernetes clouds, the controller service type depends on the host platform.

(kubernetes-bootstrap-service-type)=
When bootstrapping a controller, Juju creates a Kubernetes `Service` to expose the controller API. The `Service` type depends on the host cloud platform where the Kubernetes cluster is running:

- **`LoadBalancer`**: For managed Kubernetes on public clouds.
  - Amazon EKS (on EC2)
  - Google GKE (on GCE)
  - Microsoft AKS (on Azure)
  - Charmed Kubernetes on OpenStack
  - Charmed Kubernetes on MAAS (experimental)
- **`ClusterIP`**: For localhost and development environments.
  - MicroK8s
  - Kubernetes on LXD
  - Other/unrecognized host clouds (default)

```{note}
`LoadBalancer` creates a cloud load balancer with a public IP, while `ClusterIP` uses internal cluster networking with optional proxy access.
```
