(manage-logs)=
# How to manage logs

> See also: {ref}`log`
<!--
Make sure to also cover

logging-output:
  description: 'The logging output destination: database and/or syslog. (default "")'

It's a model config key that allows you to choose syslog, that is, not send your logsink.log to mongo

HTGs:

- Stream all the captured logs from the controller (agent and model logs)

debug-log

Cover how to filter the output: debug-log --level

- Configure the logging level (agent and model logs)

model-config --logging-config

- Configure the log file size and rotation (agent, model, and audit logs)

- Forward logs to an external logsink (agent and model logs)

- Inspect the audit log

1. SSH into a controller machine
2. View the log


- View a list of all the log files

1. SSH into a machine
2. View the files at /var/log/juju

-->



## Manage the logs

### Stream the logs

To stream the logs in the current model, run the `debug-log` command:

```text
juju debug-log
```

In a machine deployment, this will output logs for all the Juju machine and unit agents, starting with the last 10 lines, in the following format:

```text
<entity (machine or unit)> <timestamp> <log level> <Juju module> <message>
```

In Kubernetes deployments, this will not show any logs below the model level.


The command has various options that allow you to control the length, appearance, amount and type of detail.

<!--

After setting the `logging-config`, you can view the logs using the `juju debug-log` command. `debug-log` has several different `--include*` / `--exclude*` flags that you can use to filter the logs and extract the relevant log lines. These options are pretty well-covered in `juju debug-log --help`, but in short:

- `--include` / `--exclude` filters by entity (machine, unit, or application)
- `--include-module` / `--exclude-module` filters by module
- `--include-label` / `--exclude-label` filters by label

The individual `--include*` / `--exclude*` values are ORed together, and then ANDed together for each different `--include*` / `--exclude*` flag.

-->

<!--
The `--include` and `--exclude` options select and deselect, respectively, the entity that logged the message. An entity is a Juju machine or unit agent. The entity names are similar to the names shown by `juju status`.

Similarly, the `--include-module` and `--exclude-module` options can be used to influence the type of message displayed based on a (dotted) module name. The module name can be truncated.

Lastly, `--include-label` and `--exclude-label` options can be used to influence the type of message displayed based on the labels defined in logging granularity. No hash is required.

A combination of machine and unit filtering uses a logical OR whereas a combination of module and machine/unit filtering uses a logical AND.

The `--level` option places a limit on logging verbosity (e.g. `--level INFO` will allow messages of levels 'INFO', 'WARNING', and 'ERROR' to be shown).
-->


````{dropdown} Examples

To begin with the last 30 log messages:

```text
juju debug-log -n 30
```

To begin with all the log messages:

```text
juju debug-log --replay
```

To begin with the last twenty log messages for the 'lxd-pilot' model:

```text
juju debug-log -m lxd-pilot -n 20
```

To begin with the last 500 lines. The `grep` utility is used as a text filter:

```text
juju debug-log -n 500 | grep amd64
```

To begin with the last 1000 lines and exclude messages from machine 3:

```text
juju debug-log -n 1000 --exclude machine-3
```

To select all the messages emitted from a particular unit and a particular machine in the entire log:

```text
juju debug-log --replay --include unit-mysql-0 --include machine-1
```

```{important}

The unit can also be written 'mysql/0' (as shown by `juju status`).

```

To see all WARNING and ERROR messages in the entire log:

```text
juju debug-log --replay --level WARNING
```

To see all logs on the `cmr` topic (label):

```text
juju debug-log --include-label cmr
```

To progressively exclude more content from the entire log:

```text
juju debug-log --replay --exclude-module juju.state.apiserver
juju debug-log --replay --exclude-module juju.state
juju debug-log --replay --exclude-module juju
```

To begin with the last 2000 lines and include messages pertaining to both the `juju.cmd` and `juju.worker` modules:

``` bash
juju debug-log --lines 2000 \
    --include-module juju.cmd \
    --include-module juju.worker
```

````

> See more: {ref}`command-juju-debug-log`


<!--
The `debug-log` command shows the consolidated logs of all Juju agents (machines and units) running in a model. The `switch` command is used to change models. Alternatively, a model can be chosen with the '-m' option. The default model is the current model.

The 'controller' model captures logs related to internal management (controller activity) such as adding machines and services to the database. Whereas a hosted model will provide logs concerning activities that take place post- provisioning.

Due to the above, when deploying a service, it is normal for there to be an absence of logging in the workload model since logging first takes place in the 'controller' model.

The output is a fixed number of existing log lines (number specified by possible options; the default is 10) and a stream of newly appended messages. Both existing lines and appended lines can be filtered by specifying options.

The exception to the streaming is when limiting the output (option '--limit'; see below) and that limit is attained. In all other cases the command will need to be interrupted with 'Ctrl-C' in order to regain the shell prompt.



### Logging granularity

Juju logging is hierarchical and can be granular.

#### machine
```<root>``` refers to all logs related to a juju machine, including controller operation when applied to the controller model.
#### unit
```unit``` refers to all logs related to a juju unit.
#### label
```#label-name``` allows for logging based on a topic. Currently available topics are:
  * cmr: cross model relations
  * cmr-auth: cross model relations authorization
  * charmhub: dealing with the charmhub client and callers
  * http: HTTP requests
  * metrics: juju metrics, this should be used as a fallback for when prometheus isn't available.
#### module
These are modules of the juju code base. See advanced filtering for more information.
-->


### Configure the logging level

Juju saves or discards logs according to the value of the model config key `logging-config`. Therefore, `logging-config` needs to be set before the events you want to collect logs for (i.e. before attempting to reproduce a bug).

**Set values.**

- **To change the logging configuration for machine and unit agents**: <br> Run the `model-config` command with the `logging-config` key set to a `"`-enclosed, semi-colon-separated list of `<filter>=<verbosity level>` pairs.

<!--
, where the subkeys include `<root>` = the machine agent, `unit` = the unit agent, and `<label>` is a log label, and the values are log verbosity levels. For example, to change the log level of the unit agent from the default `DEBUG` to the more verbose `TRACE`, run:
-->

````{dropdown} Examples

Set machine agent logs to `WARNING` and unit agent logs to `TRACE`:

```text
juju model-config logging-config="<root>=WARNING;unit=TRACE"
```

Set unit agent logs for unit `0` of `mysql` to `DEBUG`:

```text
juju model-config logging-config="unit.mysql/0=DEBUG"
```

````


> See more: {ref}`configure-a-model`, {ref}`model-config-logging-config`

```{caution}
**To avoid filling up the database unnecessarily:**
<br>When verbose logging is no longer needed,  return logging to normal levels!
```

- **To change the logging configuration on a per-unit-agent basis:**

1.  SSH into the unit's machine. E.g., for `mysql/0`:

```text
juju ssh mysql/0
```

2. Open the unit's agent configuration file. For our example, it will be `/var/lib/juju/agents/unit-mysql-0/agent.conf`/ Then, find the `values` section, and add a line with the field `LOGGING_OVERRIDE` set to `juju=<desired log level>`, below `TRACE`. The bottom of the file should now look as below:

```text
loggingconfig: <root>=WARNING;unit=DEBUG
values:
  CONTAINER_TYPE: ""
  NAMESPACE: ""
  LOGGING_OVERRIDE: juju=trace
mongoversion: "0.0"
```

3. Restart the affected agent:

```text
sudo systemctl restart jujud-unit-mysql-0.service
```


**Get values.** To verify the current logging configuration for machine and unit agents, run `model-config` followed by the `logging-config` key:

```text
juju model-config logging-config
```

Sample output:

```text
<root>=WARNING;unit=DEBUG;#http=TRACE
```

which means that the machine agent (`<root>`) log level is set to `WARNING`, the unit agent (`unit`) log level is set at `DEBUG`, and the `http` label is set to `TRACE`.

> See more: {ref}`configure-a-model`, {ref}`model-config-logging-config`

### Forward logs to an external logsink


You can optionally forward log messages to a remote syslog server over a secure TLS connection, on a per-model basis, as below:

> See [Rsyslog documentation](http://www.rsyslog.com/doc/v8-stable/tutorials/tls_cert_summary.html) for help with security-related files (certificates, keys) and the configuration of the remote syslog server.

1. Configure the controller for remote logging by configuring it during controller creation as below:

``` bash
juju bootstrap <cloud> --config mylogconfig.yaml
```

where the YAML file is as below:

```text
syslog-host: <host>:<port>
syslog-ca-cert: |
-----BEGIN CERTIFICATE-----
 <cert-contents>
-----END CERTIFICATE-----
syslog-client-cert: |
-----BEGIN CERTIFICATE-----
 <cert-contents>
-----END CERTIFICATE-----
syslog-client-key: |
-----BEGIN PRIVATE KEY-----
 <cert-contents>
-----END PRIVATE KEY-----
```

> See more: {ref}`configure-a-controller`

2. Enable log forwarding for a model by configuring it as below:

```text
juju model-config -m <model> logforward-enabled=True
```

An initial 100 (maximum) existing log lines will be forwarded.

> See more: {ref}`configure-a-model`

````{tip}
You can configure remote logging *and* enable log forwarding on *all* the controller's models in one step by running

```text
juju bootstrap <cloud> --config mylogconfig.yaml --config logforward-enabled=True
```
````

## Manage the log files

```{caution}
Only applicable for machines -- for Kubernetes logs are written directly to `stdout` of the container and can be retrieved with native Kubernetes methods, e.g., `kubectl logs -c <container-name> <pod-name> -n <model-name>` .

```

(view-the-log-files)=
### View the log files

To view the Juju log files in a Juju machine:


1. Open a shell into the machine:

(1a) If Juju can connect to the machine (i.e., the output contains `Connected to <IP address>`) and the machine is fully provisioned, use `juju ssh`. For example, to connect to machine 0:

```text
juju ssh 0
```

(1b)  IfJuju can connect to the machine (i.e., the output contains `Connected to <IP address>`) but the machine is not fully provisioned (e.g., command hangs at `Running machine configuration script...`), use the `ssh` command followed by the address of the machine and the path to the place where Juju stores your SSH keys (including the ones it generates automatically for you):

```
ssh ubuntu@<ip-address> -i <juju-data-dir>/ssh/juju_id_rsa

```

Here, `<juju-data-dir>` defaults to `~/.local/share/juju`, but if you’ve set the `JUJU_DATA` environment variable, it will be equal to that instead.

(1c) If Juju *cannot* connect to the machine (i.e., the command never reaches `Connected to <IP address>`), use cloud-specific tools. For example, for the LXD cloud:

```text
lxc exec <container name> bash
```

or, for the MicroK8s cloud:

```text
microk8s kubectl exec controller-0 -itc api-server -n [namespace] -- bash
```

> See more: {ref}`access-a-machine-via-ssh`

2. Examine the log files under `/var/log`  with commands such as `cat`, `less`, or `tail -f`, for example:

```text
cat /var/log/juju
```

```{important}
 Which log to look at depends on the type of failure, but generally speaking, `syslog`, `cloud-init.log`, `cloud-init-output.log`, and `/var/log/juju` are good ones to look at.

```

````{dropdown} Example for a controller machine


```text
# SSH into machine 0 of the controller model:
juju ssh -m controller 0

# Navigate to the logs directory:
cd /var/log/juju

# List the contents:
ls

# View, e.g., the audit log file:
cat audit.log
```

````


### Control the log file rotation

> See also: {ref}`list-of-controller-configuration-keys`

Juju has settings to control the rotation of the various log files it produces.

There are two settings for each log file type: maximum size and number of backups. When the current log file of a particular type reaches the maximum size, Juju renames the log file to include a timestamp and gzips it, producing a "backup" log file.

Here's an example of the controller's machine agent logs with the maximum size set to 1MB, showing two timestamped backups as well as the current log file:

```text
$ juju bootstrap localhost --config agent-logfile-max-size=1MB
$ lxc exec juju-6bf629-0 -- ls -l /var/log/juju
...
-rw-r----- 1 syslog adm   3577 Jan 12 02:01 machine-0-2022-01-12T02-01-07.995.log.gz
-rw-r----- 1 syslog adm   3578 Jan 12 02:02 machine-0-2022-01-12T02-02-08.011.log.gz
-rw-r----- 1 syslog adm 600000 Jan 12 02:02 machine-0.log
```

The full list of the controller settings that configure log file rotation is shown below. Normally these are set at bootstrap time with the `--config` option (see {ref}`configure-a-controller`).

- The following config settings configure agent log files, including the API server "log sink", the machine agent logs on controller and unit machines, and the unit agent logs:

* `agent-logfile-max-backups`
* `agent-logfile-max-size`


- The following config settings configure the audit log files (note the missing "file" in the key name compared to the agent log file settings):

* `audit-log-max-backups`
* `audit-log-max-size`


- The following config settings configure the model log files:

* `model-logfile-max-backups`
* `model-logfile-max-size`

