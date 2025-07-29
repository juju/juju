(manage-controllers)=
# How to manage controllers

> See also: {ref}`controller`

```{important}
To be able to manage a controller, a user must have {ref}`controller superuser access <user-access-controller-superuser>`.
```

This document demonstrates various ways in which you can interact with a controller.

(bootstrap-a-controller)=
## Bootstrap a controller

> See also: {ref}`list-of-supported-clouds` > `<cloud name>`


To create a `juju` controller in a cloud, use the `bootstrap` command:

```{important}

**On Kubernetes:** The Juju controller needs two container images (one for the controller agent container and one for the database container). These are by default downloaded from Docker Hub, but can also be downloaded from `public.ecr.aws/juju` or `https://ghcr.io/juju` if you pass them to the `caas-image-repo` bootstrap configuration key. **We currently recommend you get them from `public.ecr.aws/juju`: `juju bootstrap mycloud --config caas-image-repo="public.ecr.aws/juju"`.**

> See more: {ref}`controller-config-caas-image-repo`. Note: While this key *can* technically be changed after bootstrap, that is only for a very specific use case (adjusting credentials used for a custom registry). For most cases it is safe to assume you can only set it during bootstrap.

```


```text
juju bootstrap
```
This will start an interactive session where you will be asked for the name of the cloud and the name you want to give the controller.

Alternatively, you can specify these things directly by adding the name of the cloud and of the controller right after the `bootstrap` command. For example, below we bootstrap a controller with the name `aws-controller` into our aws cloud:


```text
juju bootstrap aws aws-controller
```

When you use the bootstrap command in this way (non-interactively), you can also add many different options, to specify the cloud credentials to be used, to select a specific cloud region, to specify a storage pool, to constrain the controller or workload machines, to configure the deployment in various ways, to pass a cloud-specific setting, to choose a specific `juju` agent version, etc.

> See more: {ref}`command-juju-bootstrap`

```{dropdown} Tips for production

**- Machines:** Make sure to bootstrap with no less than 50 GB disk, 2 CPUs, and 4 GB RAM (e.g.,
 `juju bootstrap aws/us-east-1 mymachinecontroller --bootstrap-constraints "root-disk=50G cores=2  mem=4G"`). Bootstrapping a controller like this allows you to manage a few hundred units. However, if your needs go beyond this, consider making the controller highly available.

> See more: {ref}`manage-constraints-for-a-controller`, {ref}`make-a-controller-highly-available`

**- Kubernetes:** Juju does not currently support high-availability and backup and restore for Kubernetes controllers. Consider bootstrapping your controller on a machine cloud and then adding your Kubernetes cloud(s) to it, in a multi-cloud controller setup (`juju add-k8s myk8scloud --controller mymachinecontroller`).

> See more: {ref}`add-a-cloud`

```

````{dropdown} Tips for troubleshooting
- **Machines:**

Bootstrap on machines consists of the following steps:

1. Provision resources/a machine M from the relevant cloud, via cloud-init write a nonce file to verify we’ve found the machine we’ve provisioned.
1. Poll the newly created instance for an IP address, and attempt to connect to M.
1. Run the machine configuration script for M, which downloads, e.g., the `jujud` binaries, sets up networking, and starts jujud.

For failure at any point, retry the `bootstrap` command with the `--debug`, `--verbose`, and `keep-broken` flags:

```text
juju bootstrap <cloud> <controller> --debug --verbose --keep-broken
```


> See more: {ref}`command-juju-bootstrap`

~5% of the time bootstrap failure is due to some mirror server; in that case, retrying should succeed, and the flags won't matter. However, ~95%  of the time bootstrap failure is due to something else; in that case, `keep-broken` will ensure that the machine isn't destroyed, so you can connect to it and examine the logs.

> See more: {ref}`view-the-log-files`, {ref}`troubleshoot-your-deployment`

- **Kubernetes:**

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

> See more: {ref}`command-juju-controllers`


## View details about a controller


To view detailed information about a controller, use the `show-controller` command, optionally followed by one or more controller names. For example, below we examine a controller called `localhost-controller`:

```text
juju show-controller localhost-controller
```

By specifying various options you can also choose an output format, an output file, or get an output that includes the password for the logged in user.

> See more: {ref}`command-juju-show-controller`



## Switch to a different controller

To switch from one controller to another, use the `switch` command followed by the name of the controller. For example, below we switch to a controller called `localhost-controller-prod`:

```text
juju switch localhost-controller-prod
```

```{caution}
The `switch` command can also be used to switch to a different model. To remove any ambiguity, in some cases it may be safer to specify the model name explicitly on the template `<controller-name>:<model-name>`
```

> See more: {ref}`command-juju-switch`

(configure-a-controller)=
## Configure a controller

> See also: {ref}`configuration`, {ref}`list-of-controller-configuration-keys`
>
> See related: {ref}`configure-a-model`


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

> See more: {ref}`command-juju-bootstrap`, {ref}`command-juju-controller-config`

**Get values.** To get a controller's current configuration, run:

``` text
juju controller-config
```

This will output a list of configuration keys and their values. This will include those that were set during controller creation, inherited as a default value, or dynamically set by Juju.

> See more: {ref}`command-juju-controller-config`

(manage-constraints-for-a-controller)=
## Manage constraints for a controller

> See also: {ref}`constraint`

To manage constraints for the controller, manage them for the `controller` model or the `controller` application.

<!--Feels unnecessary and clutters.
```{important}

**Why this distinction?** <br>This distinction helps you address the fact that, while the `controller` model always contains the `controller` application, you may also deploy to it other applications (e.g.,  the `juju-dashboard` application), and their hardware needs may be different.

```
-->

```{important}

**If you want to set both types of constraints at the same time, and they are different:** <br>
You can. While the model-level constraints will apply to the entire `controller` model, the application-level constraints will make sure to override them for the `controller` application.

```


> See more:
> - {ref}`manage-constraints-for-a-model`
> - {ref}`manage-constraints-for-an-application`


## Share a controller with other users
> See also: {ref}`user`


The procedure for how to share a controller with other users depends on whether your controller is private or public.

**Share a private controller.** To share a private controller with other users:

1. Create the users.

> See more: {ref}`add-a-user`

2. Send the users the information they need to register your controller with their client and to set up their login information for the controller.

> See more: {ref}`register-a-controller`

**Share a public controller.** [TBA]


## Manage a controller's connection to the client


To add / remove details of a controller to / from your Juju client, you need to register / unregister the controller.

(register-a-controller)=
### Register a controller

```{important}

**If you are the creator of the controller:** You can skip this step. It only applies for cases where you are trying to connect to an external controller.

```

The procedure for how to register a controller with the local system varies slightly depending on whether the controller is private or public.

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

> See more: {ref}`command-juju-register`, {ref}`add-a-user`

**Register a public controller.**

```{important}

**Network requirements:** The client must be able to connect to the controller API over port `17070`.  Juju takes care of everything else. (And in most cases it takes care of this requirement too: for all clouds except for OpenStack Juju defaults to provisioning the controller with a public IP, and even for OpenStack you can choose to bootstrap with a floating IP as well.)

```

<!--
For all  clouds except for OpenStack we default to a public address by default, though you can opt out of it. For OpenStack you don’t get a public address by default but you can opt in.
-->

To register a public controller, use the  `register` command followed by the DNS host name of the public controller. For example:

```text
juju register public-controller.example.com
```

This will open a login window in your browser.

By specifying various flags you can also use this to reregister a controller or to type in your login information in your terminal rather than the browser.

> See more: {ref}`command-juju-register`

(unregister-a-controller)=
### Unregister a controller

To remove knowledge of the controller from the `juju` client, run the `unregister` command followed by the name of the controller. For example:

```text
juju unregister localhost-controller-prod
```

Note that this does not destroy the controller (though, to regain access to it, you will have to re-register it).

> See more: {ref}`command-juju-unregister`

(make-a-controller-highly-available)=
## Make a controller highly available

> See also: {ref}`high-availability`

To make a controller highly available, use the `enable-ha` command:

```{caution}
Currently only supported for controllers on a machine cloud.
```

```text
juju enable-ha
```

This will make sure that the number of controllers increases to the default minimum of 3. Sample output:

```text
maintaining machines: 0
adding machines: 1, 2
```

Optionally, you can also mention a specific controller and also the number of controller machines you want to use for HA, among other things (e.g., constraints).

```{important}
The number of controllers must be an odd number in order for a master to be "voted in" amongst its peers. A cluster with an even number of members will cause a random member to become inactive. This latter system will become a "hot standby" and automatically become active should some other member fail. Furthermore, due to limitations of the underlying database in an HA context, that number cannot exceed seven. All this means that a cluster can only have three, five, or seven **active** members.
```



If a controller is misbehaving, or if you've decided that you don't need as many controllers for HA after all, you can remove them. To remove a controller, remove its  machine from the controller model via the `remove-machine` command.


```{important}
The `enable-ha` command cannot be used to remove machines from the cluster.
```

For example, below we remove controller 1 by removing machine 1 from the controller model:

```text
juju remove-machine -m controller 1
```

```{important}
If the removal of a controller will result in an **even** number of systems then one will act as a "hot standby". <br>
If the removal of a controller will result in an **odd** number of systems then each one will actively participate in the cluster.
```


> See more: {ref}`command-juju-enable-ha`

(collect-metrics-about-a-controller)=
## Collect metrics about a controller

Each controller provides an HTTPS endpoint to expose Prometheus metrics.

> See more: [Charmhub | `juju-controller` > Endpoint metrics-endpoint > List of metrics](https://charmhub.io/juju-controller/docs/endpoint-metrics-endpoint-metrics)

To feed these metrics into Prometheus, you must first configure Prometheus to scrape them.

You can do that automatically via Juju relations or manually.

### Configure Prometheus automatically

> Available starting with Juju 3.3.
>
> Whether your controller is on machines or Kubernetes, requires a Kubernetes cloud. (That is because the required Prometheus charm is only available for Kubernetes.)
>
> If you're on a Kubernetes cloud: While it is possible to deploy Prometheus directly on the controller model, it's always best to keep your observability setup on a different model (and ideally also a different controller and a different cloud region or cloud).

To configure Prometheus to scrape the controller for metrics automatically, on a Kubernetes cloud add a model; on it, deploy `prometheus-k8s`, either directly or through the [Canonical Observability Stack](https://documentation.ubuntu.com/observability/); offer `prometheus-k8s`' `metrics-endpoint` for cross-model relations; switch to the controller model and integrate the controller application with the offer; wait until `juju status --relations` shows that everything is up and running; query Prometheus for your metric of interest / set up a Grafana dashboard and view the metrics collected by Prometheus there.


`````{dropdown} Example workflow using Prometheus and Grafana from COS

Assumes your controller application and Prometheus are on different models on the same Kubernetes cloud and that you are deploying Prometheus (`prometheus-k8s`) through the Canonical Observability Stack bundle (`cos-lite`). However, the logic would be entirely the same if they were on the same controller but different clouds (multi-cloud controller setup) or on different controllers on different clouds (except in some cases you may also have to explicitly grant access to the offer).

First, deploy COS, offer Prometheus, and integrate Prometheus with your controller.

```text
$ juju add-model observability

$ juju deploy cos-lite

$ juju status -m cos-lite --watch 1s

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

> See more:
> - [Charmhub | `juju-controller` > `metrics-endpoint | prometheus-scrape`](https://charmhub.io/juju-controller/integrations#metrics-endpoint)
> - [Charmhub | `juju-controller` > Endpoint `metrics-endpoint`: List of metrics](https://charmhub.io/juju-controller/docs/endpoint-metrics-endpoint-metrics)
>  - [Charmhub | `prometheus-k8s` > `metrics-endpoint`](https://charmhub.io/prometheus-k8s/integrations#metrics-endpoint)
> - [Charmhub | `cos-lite`](https://charmhub.io/cos-lite)
> - {ref}`switch-to-a-different-model`
> - {ref}`add-a-cross-model-relation`


### Configure Prometheus manually

> Useful if your Prometheus is outside of Juju.
>
> The Prometheus server must be able to contact the controller's API address/port `17070. (Juju controllers are usually set up to allow this automatically.)

To configure Prometheus to scrape the controller for metrics manually:

1. On the Juju side create a user for Prometheus and grant the user read access to the controller model (e.g., `juju add-user prometheus`, `juju change-user-password prometheus`, `juju grant prometheus read controller` -- where `prometheus` is just the name we've assigned to our Juju user for Prometheus).

2. Either: On the Prometheus side, configure Prometheus to skip validation. Or: On the Juju side, configure the controller to store its CA certificate in a file that Prometheus can then use to verify the server’s certificate against (`juju controller-config ca-cert > /path/to/juju-ca.crt`).

3. Add a scrape target to Prometheus by configure your `prometheus.yaml` with the following:

```{caution}
In the `username` field, the `user-` portion in front of the name we've assigned to the Juju user for Prometheus is required.
```

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

(back-up-a-controller)=
## Back up a controller


```{caution}
The procedure documented below is currently supported only for machine (non-Kubernetes) controllers.
```
(create-a-controller-backup)=
### Create a controller backup

To create a backup of a controller configuration / metadata, use the `create-backup` followed by the `-m` flag and the name of the target controller model. For example, assuming a controller called `localhost-controller`, and the standard controller model name (`controller`), we will do:

```text
juju create-backup -m localhost-controller:controller
```

```{important}
Alternatively, you can switch to the controller model and use this command without any arguments or use the `-m` flag followed by just `controller`. However, due to the delicate nature of data backups, the verbose but explicit method demonstrated above is highly recommended.

```


Sample output:

```text
backup format version: 1
juju version:          3.0.0
base:                  ubuntu@22.04

controller UUID:       ca60f7e9-647b-4460-8232-fe75749e17c7
model UUID:            a04d7604-3073-45b7-871f-030ac0360fb4
machine ID:            0
created on host:       juju-360fb4-0

checksum:              BrOGsXIK375529xlXJHX7m23Amk=
checksum format:       SHA-1, base64 encoded
size (B):              114919198
stored:                0001-01-01 00:00:00 +0000 UTC
started:               2022-11-09 09:06:46.800165238 +0000 UTC
finished:              2022-11-09 09:07:05.133077079 +0000 UTC

notes:

Downloaded to juju-backup-20221109-090646.tar.gz
```


The backup is downloaded to a default location on your computer (e.g., `/home/user`). A backup of a fresh (empty) environment, regardless of cloud type, is approximately 75 MiB in size.

The `create-backup` command also allows you to specify a custom filename for the backup file (`--filename <custom-filename`). Note: You can technically also choose to save the backup on the controller (`--no-download`), but starting with `juju v.3.0` this flag is deprecated.

> See more: {ref}`command-juju-create-backup`


### Download a controller backup

Suppose you've created a backup with the `--no-download` option, as shown below (where `controller` is the name of the controller model).

```{caution}
Starting with `juju v.3.0`, this flag is deprecated.
```

```text
$ juju create-backup -m controller --no-download
WARNING --no-download flag is DEPRECATED.

backup format version: 1
juju version:          3.0.0
base:                  ubuntu@22.04

controller UUID:       ca60f7e9-647b-4460-8232-fe75749e17c7
model UUID:            a04d7604-3073-45b7-871f-030ac0360fb4
machine ID:            0
created on host:       juju-360fb4-0

checksum:              tjqEvlspc88mYQmjV9u/m4i+prg=
checksum format:       SHA-1, base64 encoded
size (B):              114919131
stored:                0001-01-01 00:00:00 +0000 UTC
started:               2022-11-09 09:08:51.314128218 +0000 UTC
finished:              2022-11-09 09:09:10.296320799 +0000 UTC

notes:

Remote backup stored on the controller as /tmp/juju-backup-20221109-090851.tar.gz
```

As you can see from the output, this has resulted in the backup being saved remotely on the controller as `/tmp/juju-backup-20221109-090851.tar.gz`.

To download the backup, use the `download-backup` command followed by the remote location of the backup. In our case:

```text
juju download-backup /tmp/juju-backup-20221109-090851.tar.gz
```

This will output the name of the downloaded backup file. In our case:

```text
juju-backup-20221109-090851.tar.gz
```

This file will have been downloaded to a temporary location (in our case, `/home/user`).

> See more: {ref}`command-juju-download-backup`


(restore-a-controller-from-a-backup)=
### Restore a controller from a backup

To restore a controller from a backup, you can use the [stand-alone `juju-restore` tool](https://github.com/juju/juju-restore).

First, download the `juju-restore` tool and copy it to the target controller's `ha-primary` machine (typically, machine 0). To identify the primary controller machine, you can use the `juju show-controller` -- its output will list all the machines and the primary will contain `ha-primary: true`:

```text
juju show-controller
...
  controller-machines:
    "0":
      instance-id: i-073443a840f1a3626
      ha-status: ha-enabled
      ha-primary: true
    "1":
      instance-id: i-0be2c1b818e54a2ba
      ha-status: ha-enabled
    "2":
      instance-id: i-0b4705ede7d3c0faa
      ha-status: ha-enabled
...
```

Then you can copy the restore tool:

```text
# Download the latest release binary (Linux, AMD64):
wget https://github.com/juju/juju-restore/releases/latest/download/juju-restore
chmod +x juju-restore

# Switch to the controller model:
juju switch controller

# Copy juju-restore to the primary controller machine:
juju scp juju-restore 0:
```

Second, assuming that during the `create-backup` step you chose to save a local copy (the default option), use `scp` to copy the file to the same controller machine, as shown below.
```text
juju scp <path-to-backup> 0:
```

```{important}

If you've used `create-download` with the `--no-download` option, you can skip this step -- the backup is already on the primary controller machine.

```

Now, SSH into this machine and run `./juju-restore` followed by the path to the backup file, as shown below. All replica set nodes need to be healthy and in `PRIMARY` or `SECONDARY` state.


```text
# SSH into the controller machine
juju ssh 0

# Start the restore! (it will ask for confirmation)
./juju-restore <path-to-backup>
```


The `juju-restore` tool also provides several options, among which:

* `--yes`:  answer "yes" to confirmation prompts (for non-interactive mode)
* `--include-status-history`: restore the status history collection for machines and units (which can be large, and usually isn't needed)
* `--username`, `--password`, and related options: override the defaults for connecting to MongoDB
* `--allow-downgrade`: restore from a backup created with an earlier `juju` version
* `--manual-agent-control`: (in the case of restoring backups to high availability controllers) stop and restart `juju` agents and Mongo daemons on the secondary controller machines manually
* `--copy-controller`: clone the configuration of an old controller into a new controller (download the latest `juju-restore` to see this option).

For the full list of options, type: `./juju-restore --help`

> See more: [`juju-restore`](https://github.com/juju/juju-restore)


(upgrade-a-controller)=
## Upgrade a controller

The procedure depends on whether you're upgrading your controller's patch version (e.g. 2.9.25 → 2.9.48) or rather its minor or major version (e.g., 3.1 -> 3.4 or  2.9 → 3.0).

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

> See more: {ref}`upgrade-juju`, {ref}`bootstrap-a-controller`

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

> See more: {ref}`back-up-a-controller` (see esp. {ref}`create-a-controller-backup` and {ref}`restore-a-controller-from-a-backup`)

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

> See more: {ref}`migrate-a-model`, {ref}`upgrade-a-model`


```text
juju change-user-password <user> --reset
# >>> Password for "<user>" has been reset.
# >>> Ask the user to run:
# >>>     juju register
# >>> MEcTA2JvYjAWExQxMC4xMzYuMTM2LjIxNToxNzA3MAQgJCOhZjyTflOmFjl-mTx__qkvr3bAN4HAm7nxWssNDwETBnRlc3QyOQAA
# When they use this registration string, they will be prompted to create a login for the new controller.

```

> See more: {ref}`manage-a-users-login-details`


## Remove a controller

> See also: {ref}`removing-things`

There are two ways to remove a controller. Below we demonstrate each, in order of severity.


```{important}
For how to remove *knowledge* about a controller from a `juju` client, see {ref}`unregister-a-controller`
```


### Destroy a controller

A controller can be destroyed with:

`juju destroy-controller <controller-name`

You will always be prompted to confirm this action. Use the `-y` option to override this.

As a safety measure, if there are any models (besides the 'controller' model) associated with the controller you will need to pass the `--destroy-all-models` option.

Additionally, if there is persistent storage in any of the controller's models, you will be prompted to either destroy or release the storage, using the `--destroy-storage` or `--release-storage` options respectively.

For example:

```text
juju destroy-controller -y --destroy-all-models --destroy-storage aws
```

```{important}

Any model in the controller that has disabled commands will block a controller
from being destroyed. A controller administrator is able to enable all the commands across all the models in a Juju controller so that the controller can be destroyed if desired. This can be done via the {ref}`command-juju-enable-destroy-controller` command: `juju enable-destroy-controller`.

```

> See more: {ref}`command-juju-destroy-controller`


Use the `kill-controller` command as a last resort if the controller is not accessible for some reason.


### Kill a controller

The `kill-controller` command deserves some attention as it is very destructive and also has exceptional behaviour modes. This command will first attempt to remove a controller and its models in an orderly fashion. That is, it will behave like `destroy-controller`. If this fails, usually due the controller itself being  unreachable, then the controller machine and the workload machines will be destroyed by having the client contact the backing cloud's API directly.

> See more: {ref}`command-juju-kill-controller`
