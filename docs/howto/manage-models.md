(manage-models)=
# How to manage models

> See also: {ref}`model`

## Add a model

```{caution}
**If you have multiple credentials:** Be careful which one you use for the new model. Any machines subsequently on the model will be associated with this credential. As such, make sure you're not spending resources for the wrong cloud account! 
```


To add a model to the current controller using the default credential and switch to this model, run the `add-model` command followed by the name of the model. For example:

```text
juju add-model mymodel
```

You can also pass various options to choose a different controller or credential, specify a configuration, designate a different model `owner`, *not* switch to the newly create model, add it to a particular cloud (for multi-cloud controllers), etc.

> See more: {ref}`command-juju-add-model`

## View all the models available on a controller

To get a list of all the models in the current controller, use the `models` command:

```text
juju models
```

The current model will be denoted with an asterisk.

````{dropdown} Example outcome

```text
Controller: localhost-localhost

Model       Cloud/Region         Type  Status     Machines  Units  Access  Last connection
controller  localhost/localhost  lxd   available         1      1  admin   1 minute ago
prod*       localhost/localhost  lxd   available         0      -  admin   never connected
test        localhost/localhost  lxd   available         0      -  admin   2 minutes ago
```

````

By passing various options you can filter by controller, get a time stamp, output to a specific format, etc.

> See more: {ref}`command-juju-models`

(switch-to-a-different-model)=
## Switch to a different model

**Identify the current model.** To identify the current model, run the `switch` command with no arguments:

```text
juju switch
```

This will show the current controller, user, and model in a `<controller>:<user>/<model` format.

````{dropdown} Expand to see a sample output

````text
localhost-localhost:admin/test
```

````

```{important}
You can also identify the current model by running `juju models` -- your current model is the model with an asterisk!
```


**Switch to a different model.** To change from the current model to a different model, use the `switch` command followed by the target model name in a `<controller>:<user>/<model` format:

```text
juju switch <controller>:<admin>/<model>
```

The command also allows you to specify the target controller in an abbreviated form by omitting one or more of the components.

<!--
|Ways to change to a model:||
|--|--|
|`juju switch foo` | If a controller with name 'foo' exists, then this selects the last used <br> model in that controller. Otherwise, i.e. if there is no controller called 'foo', <br> then the model with name 'foo' in the current controller is selected. |
|`juju switch :foo` | Selects model 'foo' in the current controller.|
|`juju switch foo:bar` | Selects model 'bar' in controller 'foo'.|
|`juju switch foo:` | Selects the last used model in controller 'foo'.
-->

> See more: {ref}`command-juju-switch`

```{caution}
For important operations we recommend you specify the model in the unambiguous form shown above.
```


## View the status of a model


To see the status of a model and everything inside of it, run the `status` command:

```text
juju status
```

````{dropdown} Example output

```text
Model  Controller           Cloud/Region         Version  SLA          Timestamp
test   localhost-localhost  localhost/localhost  3.1.0    unsupported  16:07:52+01:00

Model "admin/test" is empty.
```

````


By passing various options you can also specify a model, see the output in color formatting or with additional sections for relations or storage, watch the status for a given duration, etc.

> See more: {ref}`command-juju-status`


## View details about a model

To view detailed information about a specific model, use the `show-model`  command followed by the model name. For example:

```text
juju show-model test
```

````{dropdown} Example output for an empty model called 'test'

```text
test:
  name: admin/test
  short-name: test
  model-uuid: 3850c8cc-0cd0-4d53-8a6d-591b63024141
  model-type: iaas
  controller-uuid: f06afa86-3461-42bb-86ed-6c2f5d7b0ac7
  controller-name: localhost-localhost
  is-controller: false
  owner: admin
  cloud: localhost
  region: localhost
  type: lxd
  life: alive
  status:
    current: available
    since: 5 hours ago
  users:
    admin:
      display-name: admin
      access: admin
      last-connection: 2 minutes ago
  sla: unsupported
  agent-version: 3.1.0
  credential:
    name: localhost
    owner: admin
    cloud: localhost
    validity-check: valid
  supported-features:
  - name: juju
    description: the version of Juju used by the model
    version: 3.1.0
```

````

By passing options you can also specify a format, an output file, etc.

> See more: {ref}`command-juju-show-model`

(configure-a-model)=
## Configure a model

> See also: {ref}`model-configuration`, {ref}`list-of-model-configuration-keys`
>
> See related: {ref}`configure-a-controller`


The procedure for how to configure a model differs slightly depending on whether you are interested in the configuration of a specific model or rather of all the models on a controller.

### Configure a specific model

**Set values.** You can set the configuration for a model both while you are creating the model and later.

- To set it for the `controller` model during control creation, use the `bootstrap` command with the `--config` option followed by the desired configuration, for example:

```text
juju bootstrap --config image-stream=daily localhost lxd-daily
```


- To set it for any other (workload) model while creating it, use the `add-model` command with the `--config` flag followed by the desired configuration:

```text
juju add-model mymodel --config image-stream=daily
```

- To set it for any model -- whether `controller` or otherwise -- after the model has already been created, use the `model-config` command followed by the desired configuration. For example, below we set the default space binding for all the applications on the model to 'myspace':

```text
juju model-config default-space=myspace
```

```{caution}

Juju does not currently check that the provided key is a valid setting, so make sure you spell it correctly.

```

In all cases, the configuration can be passed in the form of a space-separated list of key-value pairs or in the form of a YAML configuration file, and you can also use it to overwrite (e.g., with a null value) or to reset existing values, among other things.


```{important}

If you're trying to pass multiple configurations using the `--config` flag, make sure to repeat the flag for every configuration.

```

> See more:  {ref}`command-juju-bootstrap` > `--config`, {ref}`command-juju-add-model` > `config`, {ref}`command-juju-model-config`

**Get values.** You can get the configuration of a model at any time by running the `model-config` command without any argument, as below:

``` text
juju model-config
```

By using various flags of this command you can also target a specific model or key, choose a different output format, etc. 

> See more: {ref}`command-juju-model-config`

### Configure all the models on a controller

**Set values.** You can set the default configuration values for all the models on a controller either during controller creation or after.

- To set model configuration defaults during controller creation, use the `bootstrap` command with the `--model-defaults` flag followed by the desired configuration(s), for example, as below. This will affect the `controller` model and any subsequent (workload) model during controller creation.

```{important}
For the `controller` model you can override `--model-defaults` through `--config`. See more: {ref}`configure-a-controller`.
```

```text
juju bootstrap microk8s uk8s \
  --model-defaults logging-config="<root>=WARNING; unit=DEBUG" \
  --model-defaults update-status-hook-interval="60m"
```

By passing various flags you can also target a specific cloud or cloud region, pass the configuration(s) in the form of a yaml file, reset keys, etc.

> See more: {ref}`command-juju-bootstrap` > `model-defaults`

- To set model configuration defaults *after* controller creation, use the `model-defaults` command followed by the desired configuration. This willl affect any models created from that point onwards.

```text
juju model-defaults ftp-proxy=10.0.0.1:8000
```

```{important}

These defaults can be overridden, on a per-model basis, during the invocation of the `add-model` command (option `--config`) as well as by resetting specific options to their original defaults through the use of the `model-config` command (option `--reset`).

```

> See more: {ref}`command-juju-model-defaults`


**Get values.** At any point, you can get the default configuration values for all the models on a controller by running the `model-defaults` command, as below:

```text
juju model-defaults
```

Just as before, by using various flags you can filter by a specific cloud or cloud region, or see the value for a specific key, etc.

> See more: {ref}`command-juju-model-defaults`

(manage-constraints-for-a-model)=
## Manage constraints for a model
> See also: {ref}`constraint`

**Set values.** You can set constraints for the `controller` model during controller creation or to regular models at any other point. 

 ```{caution}
**To set constraints for just the `controller` application in the `controller` model *only*:** Use the `bootstrap` command with the `--bootstrap-constraints` flag. See more: {ref}`manage-constraints-for-a-controller`.
```

- To apply a constraint to the entire `controller` model during controller creation, run the `bootstrap` command with the `--constraints` option. Below we use it to ensure that every machine has 4GiB memory.

```text
juju bootstrap --constraints mem=4G aws
```

> See more: {ref}`command-juju-bootstrap`


- To set constraints for a regular model, run the `set-model-constraints` command followed by the desired key-value pair, as in the example below. This will affect all new resources provisioned for the model.


```text
juju set-model-constraints mem=4G
```

```{tip} 
To reset a constraint key to its default value, run the command with the value part empty (e.g., `juju set-model-constraints mem= `).
```

> See more: {ref}`command-juju-set-model-constraints`

**Get values.** To get constraint values for the current model, run the `model-constraints` command, as below:

```text
juju model-constraints
```

By using various flags, you can specify a model (e.g., `-m controller`, to view constraints for the controller model), an output file, etc.

> See more: {ref}`command-juju-model-constraints`


## Restrict commands on a model

**Disable commands.** To disable commands for the current model, run the `disable-command` followed by the name of the command group that you want to restrict and, optionally, a message. For example, the code below disables the ability to destroy the model and its controller:

```text
juju disable-command destroy-model ""Check with SA before destruction.""
```

<!--
If a user now attempts to destroy a protected model, they'd encounter an error similar to the following:

```text
Destroying model
ERROR cannot destroy model: Check with SA before destruction.

destroy-model operation has been disabled for the current model.
To enable the command run

    juju enable-command destroy-model
```
-->

> See more: {ref}`command-juju-disable-command`

**View a list of the disabled commands.** To see which command groups have been disabled for a model, run the `disabled-commands` command:

```text
 juju disabled-commands
```

> See more: {ref}`command-juju-disabled-commands`


**Enable commands.** To lift command restrictions, run `enable-command` followed by the command group that you want to enable. For example, the code below re-allows people to destroy the model and its controller. 

```text
juju enable-command destroy-model
```

> See more: {ref}`command-juju-enable-command`

(export-model-to-bundle)=
## Compare and export the contents of a model to a bundle

**Compare.** To compare the contents of the current model with a bundle and report any differences, run the `diff-bundle` command:

```text
juju diff-bundle <bundle>
```


````{dropdown} Expand to see an example

Consider, for example, a model for which the `status` command yields the output below:

```text
Model  Controller  Cloud/Region         Version  SLA          Timestamp
docs   lxd         localhost/localhost  2.5.0    unsupported  05:22:22Z

App        Version  Status   Scale  Charm      Store       Rev  OS      Notes
haproxy             unknown      1  haproxy    jujucharms   46  ubuntu  
mariadb    10.1.37  active       1  mariadb    jujucharms    7  ubuntu  
mediawiki  1.19.14  active       1  mediawiki  jujucharms   19  ubuntu  

Unit          Workload  Agent  Machine  Public address  Ports   Message
haproxy/0*    unknown   idle   2        10.86.33.28     80/tcp  
mariadb/0*    active    idle   1        10.86.33.192            ready
mediawiki/0*  active    idle   0        10.86.33.19     80/tcp  Ready

Machine  State    DNS           Inst id        Series  AZ  Message
0        started  10.86.33.19   juju-dbf96b-0  trusty      Running
1        started  10.86.33.192  juju-dbf96b-1  trusty      Running
2        started  10.86.33.28   juju-dbf96b-2  bionic      Running

Relation provider  Requirer              Interface     Type     Message
haproxy:peer       haproxy:peer          haproxy-peer  peer     
mariadb:cluster    mariadb:cluster       mysql-ha      peer     
mariadb:db         mediawiki:db          mysql         regular  
mediawiki:website  haproxy:reverseproxy  http          regular
```

Now say we have a bundle file `bundle.yaml` with these contents:

```text
applications:
  mediawiki:
    charm: "mediawiki"
    num_units: 1
    options:
      name: Central library
  mysql:
    charm: "mysql"
    num_units: 1
    options:
      "binlog-format": MIXED
      "block-size": 5
      "dataset-size": "512M"
      flavor: distro
      "ha-bindiface": eth0
      "ha-mcastport": 5411
      "max-connections": -1
      "preferred-storage-engine": InnoDB
      "query-cache-size": -1
      "query-cache-type": "OFF"
      "rbd-name": mysql1
      "tuning-level": safest
      vip_cidr: 24
      vip_iface: eth0
relations:
  - - "mediawiki:db"
    - "mysql:db"
```

Comparison of the currently active model with the bundle can be achieved in this way:

```text
juju diff-bundle bundle.yaml
```

This produces an output of:

```text
applications:
  haproxy:
    missing: bundle
  mariadb:
    missing: bundle
  mediawiki:
    charm:
      bundle: mediawiki-5
      model: mediawiki-19
    series:
      bundle: ""
      model: trusty
    options:
      name:
        bundle: Central library
        model: null
  mysql:
    missing: model
machines:
  "0":
    missing: bundle
  "1":
    missing: bundle
  "2":
    missing: bundle
relations:
  bundle-additions:
  - - mediawiki:db
    - mysql:db
  model-additions:
  - - haproxy:reverseproxy
    - mediawiki:website
  - - mariadb:db
    - mediawiki:db
```

This informs us of the differences in terms of applications, machines, and relations. For instance, compared to the model, the bundle is missing applications `haproxy` and `mariadb`, whereas the model is missing `mysql`. Both model and bundle utilise the 'mediawiki' application but they differ in terms of configuration. There are also differences being reported in the `machines` and `relations` sections. 

Let's now focus on the `machines` section and explore some other features of the `diff-bundle` command.

We can extend the bundle by including a bundle overlay. Consider an overlay bundle file `changes.yaml` with these machine related contents:

```text
applications:
  mediawiki:
    to: 2
  mysql:
    to: 3
machines:
  "2":
    series: trusty
    constraints: arch=amd64 cores=1
  "3":
    series: trusty
    constraints: arch=amd64 cores=1
```

Here, by means of the `--overlay` option, we can add this extra information to the comparison, effectively inflating the configuration of the bundle:

```text
juju diff-bundle bundle.yaml --overlay changes.yaml
```

This changes the `machines` section of the output to:

```text
machines:
  "0":
    missing: bundle
  "1":
    missing: bundle
  "2":
    series:
      bundle: trusty
      model: bionic
  "3":
    missing: model
```

The initial comparison displayed a lack of all three machines in the bundle. By adding machines `2` and `3` in the overlay, the output now shows machines `0` and `1` as missing in the bundle, machine `2` differs in configuration, and machine `3` is missing in the model.

As with the `deploy` command, there is the ability to map machines in the bundle to those in the model. Below, the addition of `--map-machines=2=0,3=1` makes, for the sake of the comparison, bundle machines `2` and `3` become model machines `0` and `1`, respectively:

```text
juju diff-bundle bundle.yaml --overlay changes.yaml --map-machines=2=0,3=1
```

The `machines` section now becomes:

```text
machines:
  "2":
    missing: bundle
```

The bundle shows as only missing machine `2` now, which makes sense.

The target bundle can also reside on Charmhub. In that case you would simply reference the bundle name, such as `wiki-simple`:

```text
juju diff-bundle wiki-simple
```


````

> See more: {ref}`command-juju-diff-bundle`


**Export.** To export the contents of the current model to a bundle file (a file of the form `<bundle name>.yaml`), run the `export-bundle` command with the `--filename` flag followed by the file path. For example:


```text
juju export-bundle --filename mybundle.yaml
```

The command also has flags that allow you to select a different model, include charm configuration default values in the exported bundle, etc.


````{dropdown} Example


Suppose you have a model that looks like this:

```text
$ juju status
Model        Controller  Cloud/Region        Version  SLA          Timestamp
welcome-k8s  microk8s    microk8s/localhost  3.1.6    unsupported  09:09:56+01:00

App             Version  Status  Scale  Charm           Channel  Rev  Address         Exposed  Message
example-k8s              active      1  example-k8s                1  10.152.183.43   no       
microsample-vm           active      1  microsample-vm             0  10.152.183.230  no       

Unit               Workload  Agent  Address      Ports  Message
example-k8s/0*     active    idle   10.1.64.174         
microsample-vm/0*  active    idle   10.1.64.169      
```

Running `juju export-bundle` will print this:

```text
$ juju export-bundle
bundle: kubernetes
applications:
  example-k8s:
    charm: local:example-k8s-1
    scale: 1
    constraints: arch=amd64
  microsample-vm:
    charm: local:microsample-vm-0
    scale: 1
    constraints: arch=amd64
```


````

> See more: {ref}`command-juju-export-bundle`

(upgrade-a-model)=
## Upgrade a model
> See more: {ref}`upgrading-things`

A model upgrade affects the version of Juju (Juju machine and unit agents) on all the Juju machines in the model. 

First, prepare for the upgrade:

-  Ensure the controller has already been upgraded. See more: [How to upgrade a controller <1155md`
-  Ensure the models that are to be upgraded are in good working order (`juju status`).

Then, perform the upgrade. How you upgrade a model depends on whether you'd be crossing patch versions (e.g., `v.2.9.25` -> `v.2.9.26`) or rather minor (e.g., `v.2.7` -> `v.2.8`) or major versions (`v.2` -> `v.3`). 

- To upgrade the current model across patch versions, use the `upgrade-model` command:

```text
juju upgrade-model
```

By using various flags, you can specify an agent stream, agent version, etc., or you can even perform a dry run, to simulate what would happen if you upgraded. 

```{important}

This procedure can also be used to upgrade a controller model.

```

> See more: {ref}`command-juju-upgrade-model`

- To upgrade a model's minor or major version, use model migration. First, bootstrap a controller of your target version, migrate your model to that controller, and then do `upgrade-model` on the new controller.

```{important}

This procedure cannot be used to upgrade a controller model.

```

> See more: {ref}`migrate-a-model` 



When you're done, verify that the model has been succesful by running the `status` command. If the output looks wrong, you will have to do some investigation.

`````{note}


````{dropdown} Error: some agents have not upgraded to the current model version <version>


When the running agent software that is more than 1 patch point behind the targeted upgrade version the upgrade process will abort.

One very common reason for "agent version skew" is that during a previous upgrade the agent could not be contacted and, therefore, was not upgraded along with the rest of the agents.

To overcome this situation you may force the upgrade by ignoring the agent version check:

``` text
juju upgrade-model --ignore-agent-versions
```

````

````{dropdown} Unit agent has not restarted after upgrade

It may occur that an agent does not restart upon upgrade. One thing that may help is the inspection and modification of its `agent.conf` file. Comparing it with its file before upgrading can be very useful.

Installing a different or modified configuration file will require a restart of the daemon. For example, for a machine with an ID of ‘2’:

```text
juju ssh 2 'ls -lh /etc/systemd/system/juju*'
```

This will return something similar to:

```text
-rwxr-xr-x 1 root root 326 Jun 29 19:02 /etc/systemd/system/jujud-machine-2-exec-start.sh
-rw-r--r-- 1 root root 284 Jun 29 19:02 /etc/systemd/system/jujud-machine-2.service
```

Therefore, if the agent for machine ‘2’ is not coming up you can connect to the machine in this way:

```text
juju ssh 2
```

Then modify or restore the agent file (`/var/lib/juju/agents/machine-2/agent.conf`), and while still connected to the machine, restart the agent:

```text
sudo systemctl restart jujud-machine-2
```

````

`````

(migrate-a-model)=
## Migrate a model

Model migration is the movement of a model from one controller to another. The same configuration of machines, units, and their relations will be replicated on the destination controller, while your applications continue uninterrupted. Migration is used to upgrade models across minor or major versions. Migration is also useful for load balancing: If a controller hosting multiple models reaches capacity, you can move the busiest models to a new controller, reducing load without affecting your applications.

```{important}

A controller model cannot be migrated.

```


**Prepare for migration.**

- Verify that the source and destination controllers are both known to the Juju client (i.e., they show up in the `juju controllers` output) and located in the same cloud environment.
- Verify that the version of Juju running on the destination controller is the same or newer than the version on the source controller. 
- Verify that the destination controller does not have any model with the same name as the name of the model you want to migrate to it.
- Back up the source controller.
- **If the destination controller is on a different region or VPC:** Ensure that the destination controller has direct connectivity to the source controller.
- **If the model is large:** Configure the destination controller to throttle the reconnection rate for the agents running for each machine and unit in the model and increase the migration agent timeout time.  For example: 

```text
juju controller-config agent-ratelimit-rate=50ms
juju controller-config agent-ratelimit-max=100
juju controller-config migration-agent-wait-time=30m
```

> See more: {ref}`controller-config-agent-ratelimit-rate`, {ref}`controller-config-agent-ratelimit-max`, {ref}`controller-config-migration-agent-wait-time`

<!--(Migration time depends on the complexity of the model, the resources it uses, and the capabilities of the backing cloud.)-->

- **If the model has multiple users:** Ensure that all the users have been set up on the destination controller. The operation will be aborted, and an advisory message displayed, if this is not the case.
- **If the model contains secrets:** Set up the target controller to use the same secret bank end as the source controller. For example, for a backend called `myvault`, as below. This will ensure that any secrets are correctly migrated with the model.


```text
$ juju switch sourcecontroller
$ juju show-secret-backend myvault
myvault:
  backend: vault
  config:
    endpoint: http://10.0.0.77:8200
  secrets: 0
  status: active
  id: 63c8ad37c906eb278540e942

$ juju switch targetcontroller
$ juju add-secret-backend --config /path/to/backendcfg.yaml --import-id 63c8ad37c906eb278540e942
```

**Migrate the model.** To migrate a model on the current controller to a destination controller, use the `migrate` command followed by the name of the model and the name of the destination controller, as below:

```text
juju migrate <model> <destination controller>
```

You can monitor progress from the output of the `status` command run against the source model. You may want to use a command such as `watch` to automatically refresh the status output, rather than manually running status each time:

```text
watch --color -n 1 juju status --color
```

In the output, a 'Notes' column is appended to the model overview line at the top of the output. The migration will step through various states, from 'starting' to 'successful'.

The 'status' section in the output from the `show-model` command also includes details on the current or most recently run migration. It adds extra information too, such as the migration start time, and is a good place to start if you need to determine why a migration has failed.

This section will look similar to the following after starting a migration:

```text
status:

current: available

since: 23 hours ago

migration: uploading model binaries into destination controller

migration-start: 21 seconds ago
```

Migration time depends on the complexity of the model, the resources it uses, and the capabilities of the backing cloud.

If failure occurs during the migration process, the model, in its original state, will be reverted to the original controller.

When the migration has completed successfully, the model will no longer reside on the source controller. It, and its applications, machines and units, will be running on the destination controller.

Inspect the migrated model with the `status` command:

```text
juju status -m <destination-controller>:<model>
```



<!-- MIGRATING LARGE MODELS:
When a model is migrated, the agents running for each machine and unit need to reestablish a connection to the new controller. If the model is large, it may be that the number and frequency of incoming connections is enough to overload the controller. There are 2 controller config settings which can be used to throttle the agent reconnection rate.

* agent-ratelimit-max - the number of agents allowed to connect before rate limiting kicks in
* agent-ratelimit-rate - the minimum time interval between connections when rate limiting is active

The default values for these config attributes are:

* agent-ratelimit-max = 10
* agent-ratelimit-rate = 250ms

When migrating large models, with high performing controllers, these values may be better, in order to address pre- or post- check errors that may be reported:

* agent-ratelimit-max = 100
* agent-ratelimit-rate = 50ms

For example:

```text
juju controller-config agent-ratelimit-rate=50ms
juju controller-config agent-ratelimit-max=100
```

-->

<!--
<a href="#heading---migrating-models-with-secrets"><h3 id="heading---migrating-models-with-secrets"> Migrating models with secrets</h3></a>


If a model has secrets stored in a secret backend like Vault, migrating that model to a new controller requires an extra step. The target controller must be set up to use the same (Vault) backend as is available on the source controller. This is done by running `add-secret-backend` on the target controller and using the `import-id` option to use the same internal backend ID as on the source controller.

On the source controller, inspect the backend(s) in use by the model to be migrated. The relevant backends can be discovered by running `juju show-model` as described earlier. For any in-use backends, use the `show-secret-backend` command (or just list them all with `--format yaml`) to see the ID of the relevant backend(s). Before running the migration, add the backend(s) to the target controller, eg

```text
$ juju switch sourcecontroller
$ juju show-secret-backend myvault
myvault:
  backend: vault
  config:
    endpoint: http://10.0.0.77:8200
  secrets: 0
  status: active
  id: 63c8ad37c906eb278540e942

$ juju switch targetcontroller
$ juju add-secret-backend --config /path/to/backendcfg.yaml --import-id 63c8ad37c906eb278540e942
```

Now a migration can be run as normal and any secrets will be correctly migrated with the model.

-->


````{note}

```{dropdown} Error: migration: 'aborted, removing model from target controller: model data transfer failed, failed to import model into target controller: granting admin permission to the owner: user "<user>" is permanently deleted'


This error occurs when the model owner does not exist on the target controller. The solution is to create a user with that name on the target controller. 

**Note:** The underlying cause is because a model is tightly coupled with the user who has created it. Starting with Juju 4, it will be possible to identify models independently of the user. 


```

```{dropdown} Error:migration: 'aborted, removing model from target controller: model data transfer failed, failed to import model into target controller: credential "<credential>" not found (not found)'


This error occurs when the model owner does not own the credential associated with the model. The solution is to change the credential to a credential the user owns (via `juju set-credential`).


```

```{dropdown} Error: migration: 'aborted, removing model from target controller: machine sanity check failed, 1 error found'


This error occurs when the machines known by Juju differ from the ones the underlying cloud reports (e.g., a LXD cloud still sees a container that has been removed from Juju). The solution is to check the cloud and resolve the difference (i.e., continuing with the previous example, to delete the container from the LXD cloud as well). 


```

````

> See more: {ref}`command-juju-migrate`

## Destroy a model
> See also: {ref}`removing-things`

To remove a model, along with any associated machines and applications, use the `destroy-model` command followed by the name of the model:

```text
juju destroy-model <model>
```

The command has a variety of flags that you can use to skip the confirmation, to rush through the destruction without waiting for each step to complete, to release or destroy any persistant storage on the model, etc., or even to force destroy the model, ignoring any errors (not recommended as it might leave behind unresolved issues).

> See more: {ref}`command-juju-destroy-model`

