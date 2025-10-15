(manage-controllers)=
# How to manage controllers

```{ibnote}
See also: {ref}`controller`
```

A user with controller {ref}`user-access-controller-superuser` access can manage the controller in every way from bootstrap to removal, and can also create users and give them access to the controller or to entities within the scope of the controller.

(bootstrap-a-controller)=
## Bootstrap a controller

Given a cloud you've already added to Juju, to bootstrap a Juju controller in that, use the `bootstrap` command followed by the name of the cloud and the name you want to assign to your new controller. For example:

```text
juju bootstrap aws aws-controller
```

You can also add many different options, to specify the cloud credentials to be used, to select a specific cloud region, to specify a storage pool, to constrain the controller or workload machines, to configure the deployment in various ways, to pass a cloud-specific setting, to choose a specific `juju` agent version, etc.

```{ibnote}
See more: {ref}`command-juju-bootstrap`, {ref}`cloud-specific reference docs <list-of-supported-clouds>`, {ref}`list-of-constraints`, {ref}`list-of-controller-configuration-keys`
```

````{dropdown} Recommended configuration - Kubernetes
The Juju controller needs two container images (one for the controller agent container and one for the database container). These are by default downloaded from Docker Hub, but can also be downloaded from `public.ecr.aws/juju` or `https://ghcr.io/juju` if you pass them to the {ref}`controller-config-caas-image-repo` bootstrap configuration key. We currently recommend you get them from `public.ecr.aws/juju`, as below:

```text
juju bootstrap mycloud --config caas-image-repo="public.ecr.aws/juju"
```

Note: While the {ref}`controller-config-caas-image-repo` *can* technically be changed after bootstrap, that is only for a very specific use case (adjusting credentials used for a custom registry). For most cases it is safe to assume you can only set it during bootstrap.

````

````{dropdown} Tips for production - machines
Make sure to bootstrap with no less than 50 GB disk, 2 CPUs, and 4 GB RAM (e.g., `juju bootstrap aws/us-east-1 mymachinecontroller --bootstrap-constraints "root-disk=50G cores=2  mem=4G"`). Bootstrapping a controller like this allows you to manage a few hundred units. However, if your needs go beyond this, consider making the controller highly available.

```{ibnote}
See more: {ref}`manage-constraints-for-a-controller`, {ref}`make-a-controller-highly-available`
```
````
````{dropdown} Tips for production - Kubernetes
Juju does not currently support high-availability and backup and restore for Kubernetes controllers. Consider bootstrapping your controller on a machine cloud and then adding your Kubernetes cloud(s) to it, in a multi-cloud controller setup (`juju add-k8s myk8scloud --controller mymachinecontroller`).

```{ibnote}
See more: {ref}`add-a-cloud`
```
````

````{dropdown} Tips for troubleshooting - machines
Bootstrap on machines consists of the following steps:

1. Provision resources/a machine M from the relevant cloud, via cloud-init write a nonce file to verify we’ve found the machine we’ve provisioned.
1. Poll the newly created instance for an IP address, and attempt to connect to M.
1. Run the machine configuration script for M, which downloads, e.g., the `jujud` binaries, sets up networking, and starts jujud.

For failure at any point, retry the `bootstrap` command with the `--debug`, `--verbose`, and `keep-broken` flags:

```text
juju bootstrap <cloud> <controller> --debug --verbose --keep-broken
```

```{ibnote}
See more: {ref}`command-juju-bootstrap`
```

~5% of the time bootstrap failure is due to some mirror server; in that case, retrying should succeed, and the flags won't matter. However, ~95%  of the time bootstrap failure is due to something else; in that case, `keep-broken` will ensure that the machine isn't destroyed, so you can connect to it and examine the logs.

```{ibnote}
See more: {ref}`view-the-log-files`, {ref}`troubleshoot-your-deployment`
```

````
````{dropdown} Tips for troubleshooting - Kubernetes
Bootstrap on Kubernetes includes creating a Kubernetes pod called `controller-0` containing a container called `api-server`. Matching this, the output of the `juju bootstrap` command includes `Creating k8s resources for controller <namespace>`, where `<namespace>` is something like `controller-foobar`. To troubleshoot, inspect this `api-server` container with `kubectl`:

```text
kubectl exec controller-0 -itc api-server -n <namespace> -- bash
```
````

## View all the known controllers

To see a list of all the controllers known to the `juju` client, run the `controllers` command:

```text
juju controllers
```

Sample output for a case where there is just a single controller boostrapped into the `localhost` cloud:


```text
Use --refresh option with this command to see the latest information.

Controller             Model       User   Access     Cloud/Region         Models  Nodes    HA  Version
localhost-controller*  controller  admin  superuser  localhost/localhost       1      1  none  3.0.0
```

By specifying various options you can also choose a specific output format, an output file, etc.

```{ibnote}
See more: {ref}`command-juju-controllers`
```

## View details about a controller

To view detailed information about a controller, use the `show-controller` command, optionally followed by one or more controller names. For example, below we examine a controller called `localhost-controller`:

```text
juju show-controller localhost-controller
```

By specifying various options you can also choose an output format, an output file, or get an output that includes the password for the logged in user.

```{ibnote}
See more: {ref}`command-juju-show-controller`
```

## Switch to a different controller

To switch from one controller to another, use the `switch` command followed by the name of the controller. For example, below we switch to a controller called `localhost-controller-prod`:

```text
juju switch localhost-controller-prod
```

```{ibnote}
See more: {ref}`command-juju-switch`
```

(configure-a-controller)=
## Configure a controller

```{ibnote}
See also: {ref}`configuration`, {ref}`list-of-controller-configuration-keys`

See related: {ref}`configure-a-model`
```

**Set values.**
A controller configuration key can be assigned a value during controller-creation time or post-creation time. The vast majority of keys are set in the former way.

- To set a controller's configuration at controller-creation time, use the `bootstrap` command with the `--config`  followed by the relevant `<key>=<value` pair(s). For example, the code below creates a controller `localhost` on a cloud `lxd` and at the same time configures the controller such that  the `bootstrap-timeout` key is 700 seconds:

``` text
juju bootstrap --config bootstrap-timeout=700 localhost lxd
```

- To set a controller's configuration once it's already been created, use the `controller-config` command followed by the relevant `<key>=<value` pair(s). For example, the code below configures an existing controller named `aws` so as to record auditing information, with the number of old audit log files to keep being set at 5.

``` text
juju controller-config -c aws auditing-enabled=true audit-log-max-backups=5
```

```{ibnote}
See more: {ref}`command-juju-bootstrap`, {ref}`command-juju-controller-config`
```

**Get values.** To get a controller's current configuration, run:

``` text
juju controller-config
```

This will output a list of configuration keys and their values. This will include those that were set during controller creation, inherited as a default value, or dynamically set by Juju.

```{ibnote}
See more: {ref}`command-juju-controller-config`
```

(manage-constraints-for-a-controller)=
## Manage constraints for a controller

```{ibnote}
See also: {ref}`constraint`
```

To manage constraints for the controller, manage them for the `controller` model or the `controller` application.

If you want to set both types of constraints at the same time, and they are different: You can. While the model-level constraints will apply to the entire `controller` model and anything it contains, the application-level constraints will override them for the `controller` application.


```{ibnote}
See more: {ref}`manage-constraints-for-a-model`, {ref}`manage-constraints-for-an-application`
```

## Share a controller with other users

```{ibnote}
See also: {ref}`user`
```

The procedure for how to share a controller with other users depends on whether your controller is private or public.

**Share a private controller.** To share a private controller with other users:

1. Create the users.

```{ibnote}
See more: {ref}`add-a-user`
```

2. Send the users the information they need to register your controller with their client and to set up their login information for the controller.

```{ibnote}
See more: {ref}`register-a-controller`
```

**Share a public controller.** [TBA]

## Manage a controller's connection to the client

To add / remove details of a controller to / from your Juju client, you need to register / unregister the controller.

(register-a-controller)=
### Register a controller

The procedure for how to make an external controller known to your local client varies slightly depending on whether the controller is private or public.

**Register a private controller.** To register a private controller, use the `register` command followed by your unique registration key -- that is, copy-paste and run the line of code provided to you by the person who has added you to the controller via the `juju add-user` command. For example:

```text
juju register MFATA3JvZDAnExMxMDQuMTU0LjQyLjQ0OjE3MDcwExAxMC4xMjguMC4yOjE3MDcwBCBEFCaXerhNImkKKabuX5ULWf2Bp4AzPNJEbXVWgraLrAA=

```

This will start an interactive session prompting you to supply a local name for the controller as well as a username and a password for you as a new `juju` user on the controller.

````{dropdown} Example session

Admin adding a new user 'alex' to the controller:

```text
# Add a user named `alex`:
$ juju add-user alex
User "alex" added
Please send this command to alex:
    juju register MFUTBGFsZXgwFRMTMTAuMTM2LjEzNi4xOToxNzA3MAQghBj6RLW5VgmCSWsAesRm5unETluNu1-FczN9oVfNGuYTFGxvY2FsaG9zdC1jb250cm9sbGVy

"alex" has not been granted access to any models. You can use "juju grant" to grant access.
```

New user 'alex' accessing the controller:

```text
$ juju register MFUTBGFsZXgwFRMTMTAuMTM2LjEzNi4xOToxNzA3MAQghBj6RLW5VgmCSWsAesRm5unETluNu1-FczN9oVfNGuYTFGxvY2FsaG9zdC1jb250cm9sbGVy
Enter a new password: ********
Confirm password: ********
Enter a name for this controller [localhost-controller]: localhost-controller
Initial password successfully set for alex.

Welcome, alex. You are now logged into "localhost-controller".

There are no models available. You can add models with
"juju add-model", or you can ask an administrator or owner
of a model to grant access to that model with "juju grant".

```
````

The command also has a flag that allows you to overwrite existing information, for cases where you need to reregister a controller.

```{ibnote}
See more: {ref}`command-juju-register`, {ref}`add-a-user`
```

**Register a public controller.**

First, check that your public controller meets the prerequisites: Your client must be able to connect to the controller API over port `17070`. Note: Juju takes care of everything else, and in most cases it takes care of this requirement too: for all clouds except for OpenStack Juju defaults to provisioning the controller with a public IP, and even for OpenStack you can choose to bootstrap with a floating IP as well.

Then, to register the public controller, use the  `register` command followed by the DNS host name of the public controller. For example:

```text
juju register public-controller.example.com
```

This will open a login window in your browser.

By specifying various flags you can also use this to reregister a controller or to type in your login information in your terminal rather than the browser.

```{ibnote}
See more: {ref}`command-juju-register`
```

(unregister-a-controller)=
### Unregister a controller

To remove knowledge of the controller from the `juju` client, run the `unregister` command followed by the name of the controller. For example:

```text
juju unregister localhost-controller-prod
```

Note that this does not destroy the controller (though, to regain access to it, you will have to re-register it).

```{ibnote}
See more: {ref}`command-juju-unregister`
```

(make-a-controller-highly-available)=
## Make a controller highly available

```{ibnote}
See also: {ref}`high-availability`
```
```{important}
Currently only supported for controllers on a machine cloud.
```

To make a controller highly available,  add a unit using the `juju add-unit` command:

```text
juju add-unit -m controller controller -n 2
```

This will make sure that the number of controllers increases to the default minimum of 3. Sample output:

```text
maintaining machines: 0
adding machines: 1, 2
```

Optionally, you can also mention a specific controller and also the number of controller machines you want to use for HA, among other things (e.g., constraints). Note: The number of controllers must be an odd number in order for a master to be "voted in" amongst its peers. (A cluster with an even number of members will cause a random member to become inactive, though that member will remain on "hot standby" and automatically become active should some other member fail.) Furthermore, due to limitations of the underlying database in an HA context, that number cannot exceed seven. (Any member in excess of seven will become inactive.Thus, a cluster can only have three, five, or seven **active** members.)

If a controller is misbehaving, or if you've decided that you don't need as many controllers for HA after all, you can remove it either by removing a unit or its host machine. For example, below we remove controller 1 by removing machine 1 from the controller model:

```text
juju remove-machine -m controller 1
```

```{ibnote}
See more: {ref}`manage-units`, {ref}`manage-machines`
```

(collect-metrics-about-a-controller)=
## Collect metrics about a controller

Each controller provides an HTTPS endpoint to expose Prometheus metrics.

```{ibnote}
See more: [Charmhub | `juju-controller` > Endpoint metrics-endpoint > List of metrics](https://charmhub.io/juju-controller/docs/endpoint-metrics-endpoint-metrics)
```

To feed these metrics into Prometheus, you must first configure Prometheus to scrape them.

You can do that automatically via Juju relations or manually.

### Configure Prometheus automatically

```{versionadded} 3.3
```

```{important}
As the required Prometheus charm is only available for Kubernetes, this option requires a Kubernetes cloud.

If you're already on a Kubernetes cloud: While it is possible to deploy Prometheus directly on the controller model, it's always best to keep your observability setup on a different model (and ideally also a different controller and a different cloud region or cloud).
```

To configure Prometheus to scrape the controller for metrics automatically, on a Kubernetes cloud add a model; on it, deploy `prometheus-k8s`, either directly or through the [Canonical Observability Stack](https://documentation.ubuntu.com/observability/); offer `prometheus-k8s`' `metrics-endpoint` for cross-model relations; switch to the controller model and integrate the controller application with the offer; wait until `juju status --relations` shows that everything is up and running; query Prometheus for your metric of interest / set up a Grafana dashboard and view the metrics collected by Prometheus there.


`````{dropdown} Example workflow using Prometheus and Grafana from COS

Assumes your controller application and Prometheus are on different models on the same Kubernetes cloud and that you are deploying Prometheus (`prometheus-k8s`) through the Canonical Observability Stack bundle (`cos-lite`). However, the logic would be entirely the same if they were on the same controller but different clouds (multi-cloud controller setup) or on different controllers on different clouds (except in some cases you may also have to explicitly grant access to the offer).

First, deploy COS, offer Prometheus, and integrate Prometheus with your controller.

```text
$ juju add-model observability

$ juju deploy cos-lite

$ watch -n 1 -c juju status -m cos-lite --color

$ juju offer prometheus:metrics-endpoint

$ juju switch controller

$  juju integrate controller admin/observability.prometheus

$  juju status --relations

```

Now, either query Prometheus directly or set up a Grafana dashboard and view the metrics there.

````{dropdown} Query Prometheus directly

Use `curl` on the pattern `<Prometheus_unit_IP_address>:9090api/v1/query?query=<metric>`. For example:

```text
$  curl 10.1.170.185:9090/api/v1/query?query=juju_apiserver_request_duration_seconds
```

````

````{dropdown} View metrics in a Grafana dashboard

On the observability model, use the Grafana charm's `get-admin-password` to generate an admin password:


```text
$ juju switch observability
$ juju run grafana/0 get-admin-password
# Example output:
Running operation 1 with 1 task
  - task 2 on unit-grafana-0

Waiting for task 2...
admin-password: 0OpLUlxJXQaU
url: http://10.238.98.110/observability-grafana
```

On your local machine, open a browser window and copy-paste the Grafana URL. In the username field, enter 'admin'. In the password field, enter the `admin-password`. If everything has gone well, you should now be logged in.

On the new screen, in the top-right, click on the Menu icon, then **Dashboards**. Then, on the new screen, in the top-left, click on **New**, **Upload dashboard JSON file**, and upload your Grafana dashboard definition file, for example, the JSON Grafana-dashboard-definition file below; then, in the IL3-2 field, from the dropdown, select the suggested `juju_observability...` option.

[Juju Controllers-1713888589960.json|attachment](https://discourse.charmhub.io/uploads/short-url/yOxvgum6eo3NmMxPaTRKLOLmbo0.json) (200.9 KB)


On the new screen, at the very top, expand the Juju Metrics section and inspect the results.

Make a change to your controller (e.g., run `juju add-model test` to add another model and trigger some more API server connections) and refresh the page to view the updated results!

````
`````

```{ibnote}
See more:
- [Charmhub | `juju-controller` > `metrics-endpoint | prometheus-scrape`](https://charmhub.io/juju-controller/integrations#metrics-endpoint)
- [Charmhub | `juju-controller` > Endpoint `metrics-endpoint`: List of metrics](https://charmhub.io/juju-controller/docs/dpoint-metrics-endpoint-metrics)
 - [Charmhub | `prometheus-k8s` > `metrics-endpoint`](https://charmhub.io/prometheus-k8s/integrations#metrics-endpoint)
- [Charmhub | `cos-lite`](https://charmhub.io/cos-lite)
- {ref}`switch-to-a-different-model`
- {ref}`add-a-cross-model-relation`
```


### Configure Prometheus manually

```{tip}
Useful if your Prometheus is outside of Juju.
```

```{important}
The Prometheus server must be able to contact the controller's API address/port `17070`. (Juju controllers are usually set up to allow this automatically.)
```

To configure Prometheus to scrape the controller for metrics manually:

1. On the Juju side create a user for Prometheus and grant the user read access to the controller model (e.g., `juju add-user prometheus`, `juju change-user-password prometheus`, `juju grant prometheus read controller` -- where `prometheus` is just the name we've assigned to our Juju user for Prometheus).

2. Either: On the Prometheus side, configure Prometheus to skip validation. Or: On the Juju side, configure the controller to store its CA certificate in a file that Prometheus can then use to verify the server’s certificate against (`juju controller-config ca-cert > /path/to/juju-ca.crt`).

3. Add a scrape target to Prometheus by configure your `prometheus.yaml` with the following:

```text
scrape_configs:
  job_name: juju
    metrics_path: /introspection/metrics
    scheme: https
    static_configs:
      targets: {ref}`'<controller-address>:17070']
    basic_auth:
      username: user-<name of Juju user for Prometheus, e.g., 'prometheus'>
      password: <password of Juju user for Prometheus>
    tls_config:
      ca_file: /path/to/juju-ca.crt
```

(upgrade-a-controller)=
## Upgrade a controller

The procedure depends on whether you're upgrading your controller's patch version (e.g. `2.9.25` &rarr; `2.9.48`) or rather its minor or major version (e.g., `3.1` &rarr; `3.4` or  `2.9` &rarr; `3.0`).

(upgrade-a-controllers-patch-version)=
### Upgrade a controller's patch version

To upgrade your controller's patch version, on the target controller, use the `juju upgrade-controller` command with the `--agent-version` flag followed by the desired patch version (of the same major and minor):

```text
juju upgrade-controller --agent-version <current major. current minor. target patch>
```

For example, assuming a controller version `3.0.0`, to upgrade to `3.0.2`:

```text
juju upgrade-controller --agent-version 3.0.2
```

(upgrade-a-controllers-minor-or-major-version)=
### Upgrade a controller's minor or major version


It is not possible to upgrade a controller's minor or major version. Instead, you should:

1. Use a client upgraded to the desired version to bootstrap a new controller of that version. For example:

```text
snap refresh juju --channel=<target controller version>
juju bootstrap <cloud> newcontroller
```

```{ibnote}
See more: {ref}`upgrade-juju`, {ref}`bootstrap-a-controller`
```

2. Recreate your old controller's configuration (settings, users, clouds, and models) in the new controller (on machine clouds, through our dedicated tools for backup and restore). For example:

```text
# Create a backup of the old controller's controller model
# and make note of the path to the backup file:
juju create-backup -m oldcontroller:controller
# Sample output:
# >>> ...
# >>>  Downloaded to juju-backup-20221109-090646.tar.gz

# Download the stand-alone juju-restore tool:
wget https://github.com/juju/juju-restore/releases/latest/download/juju-restore
chmod +x juju-restore

# Switch to the new controller's controller model:
juju switch newcontroller:controller

# Copy the juju-restore tool to the primary controller machine:
juju scp juju-restore 0:

# Copy the backup file to the primary controller machine:
juju scp <path to backup> 0:

# SSH into the primary controller machine:
juju ssh 0

# Start the restore with the '--copy-controller' flag:
./juju-restore --copy-controller <path to backup>
# Congratulations, your <old version> controller config has been cloned into your <new version> controller.
```

```{ibnote}
See more: {ref}`back-up-a-controller` (see esp. {ref}`create-a-controller-backup` and {ref}`restore-a-controller-from-a-backup`)
```

3. Migrate your models from the old controller to the new, then upgrade them to match the new controller's version.


```text
# Switch to the old controller:
juju switch oldcontroller

# Migrate your models to the new controller:
juju migrate <model> newcontroller

# Switch to the new controller:
juju switch newcontroller

# Upgrade the migrated models to match the new controller's agent version:
juju upgrade-model --agent-version=<new controller's agent version>
```

```{ibnote}
See more: {ref}`migrate-a-model`, {ref}`upgrade-a-model`
```

```text
juju change-user-password <user> --reset
# >>> Password for "<user>" has been reset.
# >>> Ask the user to run:
# >>>     juju register
# >>> MEcTA2JvYjAWExQxMC4xMzYuMTM2LjIxNToxNzA3MAQgJCOhZjyTflOmFjl-mTx__qkvr3bAN4HAm7nxWssNDwETBnRlc3QyOQAA
# When they use this registration string, they will be prompted to create a login for the new controller.

```

```{ibnote}
See more: {ref}`manage-a-users-login-details`
```

## Remove a controller

```{ibnote}
See also: {ref}`removing-things`
```

There are two ways to remove a controller. Below we demonstrate each, in order of severity.

```{note}
For how to remove *knowledge* about a controller from a `juju` client, see {ref}`unregister-a-controller`
```


### Destroy a controller

```{important}

Any model in the controller that has disabled commands will block a controller
from being destroyed. A controller administrator is able to enable all the commands across all the models in a Juju controller so that the controller can be destroyed if desired. This can be done via the {ref}`command-juju-enable-destroy-controller` command: `juju enable-destroy-controller`.
```

A controller can be destroyed with:

`juju destroy-controller <controller-name`

You will always be prompted to confirm this action. Use the `-y` option to override this.

As a safety measure, if there are any models (besides the 'controller' model) associated with the controller you will need to pass the `--destroy-all-models` option.

Additionally, if there is persistent storage in any of the controller's models, you will be prompted to either destroy or release the storage, using the `--destroy-storage` or `--release-storage` options respectively.

For example:

```text
juju destroy-controller -y --destroy-all-models --destroy-storage aws
```

```{ibnote}
See more: {ref}`command-juju-destroy-controller`
```

Use the `kill-controller` command as a last resort if the controller is not accessible for some reason.


### Kill a controller

The `kill-controller` command deserves some attention as it is very destructive and also has exceptional behaviour modes. This command will first attempt to remove a controller and its models in an orderly fashion. That is, it will behave like `destroy-controller`. If this fails, usually due the controller itself being  unreachable, then the controller machine and the workload machines will be destroyed by having the client contact the backing cloud's API directly.

```{ibnote}
See more: {ref}`command-juju-kill-controller`
```
