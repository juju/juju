(command-juju-add-k8s)=
# `juju add-k8s`

```
Usage: juju add-k8s [options] <k8s name>

Summary:
Adds a k8s endpoint and credential to Juju.

Global Options:
--debug  (= false)
    equivalent to --show-log --logging-config=<root>=DEBUG
-h, --help  (= false)
    Show help on a command or other topic.
--logging-config (= "")
    specify log levels for modules
--quiet  (= false)
    show no informational output
--show-log  (= false)
    if set, write the log file to stderr
--verbose  (= false)
    show more verbose output

Command Options:
-B, --no-browser-login  (= false)
    Do not use web browser for authentication
--aks  (= false)
    used when adding an AKS cluster
-c, --controller (= "")
    Controller to operate in
--client  (= false)
    Client operation
--cloud (= "")
    k8s cluster cloud
--cluster-name (= "")
    Specify the k8s cluster to import
--context-name (= "")
    Specify the k8s context to import
--credential (= "")
    the credential to use when accessing the cluster
--eks  (= false)
    used when adding an EKS cluster
--gke  (= false)
    used when adding a GKE cluster
--local  (= false)
    DEPRECATED (use --client): Local operation only; controller not affected
--project (= "")
    project to which the cluster belongs
--region (= "")
    k8s cluster region or cloud/region
--resource-group (= "")
    the Azure resource group of the AKS cluster
--skip-storage  (= false)
    used when adding a cluster that doesn't have storage
--storage (= "")
    k8s storage class for workload storage

Details:
Creates a user-defined cloud based on a k8s cluster.

The new k8s cloud can then be used to bootstrap into, or it
can be added to an existing controller.

Use --controller option to add k8s cloud to a controller.
Use --client option to add k8s cloud to this client.

Specify a non default kubeconfig file location using $KUBECONFIG
environment variable or pipe in file content from stdin.

The config file can contain definitions for different k8s clusters,
use --cluster-name to pick which one to use.
It's also possible to select a context by name using --context-name.

When running add-k8s the underlying cloud/region hosting the cluster needs to be
detected to enable storage to be correctly configured. If the cloud/region cannot
be detected automatically, use either
  --cloud <cloudType|cloudName> to specify the host cloud
or
  --region <cloudType|cloudName>/<someregion> to specify the host
  cloud type and region.

Region is strictly necessary only when adding a k8s cluster to a JAAS controller.
When using a standalone Juju controller, usually just --cloud is required.

Once Juju is aware of the underlying cloud type, it looks for a suitably configured
storage class to provide operator and workload storage. If none is found, use
of the --storage option is required so that Juju will create a storage class
with the specified name.

If the cluster does not have a storage provisioning capability, use the
--skip-storage option to add the cluster without any workload storage configured.

When adding a GKE or AKS cluster, you can use the --gke or --aks option to
interactively be stepped through the registration process, or you can supply the
necessary parameters directly.

Examples:
    juju add-k8s myk8scloud
    juju add-k8s myk8scloud --client
    juju add-k8s myk8scloud --controller mycontroller
    juju add-k8s --context-name mycontext myk8scloud
    juju add-k8s myk8scloud --region cloudNameOrCloudType/someregion
    juju add-k8s myk8scloud --cloud cloudNameOrCloudType
    juju add-k8s myk8scloud --cloud cloudNameOrCloudType --region=someregion
    juju add-k8s myk8scloud --cloud cloudNameOrCloudType --storage mystorageclass

    KUBECONFIG=path-to-kubeconfig-file juju add-k8s myk8scloud --cluster-name=my_cluster_name
    kubectl config view --raw | juju add-k8s myk8scloud --cluster-name=my_cluster_name

    juju add-k8s --gke myk8scloud
    juju add-k8s --gke --project=myproject myk8scloud
    juju add-k8s --gke --credential=myaccount --project=myproject myk8scloud
    juju add-k8s --gke --credential=myaccount --project=myproject --region=someregion myk8scloud

    juju add-k8s --aks myk8scloud
    juju add-k8s --aks --cluster-name mycluster myk8scloud
    juju add-k8s --aks --cluster-name mycluster --resource-group myrg myk8scloud

See also:
    remove-k8s

```