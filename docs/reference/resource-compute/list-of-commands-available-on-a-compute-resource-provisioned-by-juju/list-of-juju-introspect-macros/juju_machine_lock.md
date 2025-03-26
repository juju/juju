(juju_machine_lock)=
# `juju_machine_lock`


This function actually calls into every agent on the machine to ask about the agent's view of the hook execution lock. Where the {ref}`machine-lock.log <logfile-varlogjujumachine-locklog>` file shows the history of the machine lock, the introspection endpoint shows the current status of the lock, whether the agent holds the lock, or is waiting for the lock.

During a deploy of `hadoop-kafka`, after the machine 0 has started, and is deploying the two units, we can see the following:

```text
machine-0:
  holder: none
unit-namenode-0:
  holder: uniter (run install hook), holding 1m42s
unit-resourcemanager-0:
  holder: none
  waiting:
  - uniter (run install hook), waiting 1m41s
```
You can see that the `namenode/0` unit has the uniter worker holding the hook, and it is running the install hook, and at the time of executing the `juju_machine_lock` command it had been holding the lock for one minute and 42 seconds.

You can additionally see that the `resourcemanager/0` unit is waiting to run its install hook.

As the installation progresses, the subordinate units are deployed, and the output looks more like this:

```text
machine-0:
  holder: none
unit-ganglia-node-7:
  holder: none
  waiting:
  - uniter (run install hook), waiting 1s
unit-ganglia-node-8:
  holder: none
unit-namenode-0:
  holder: uniter (run relation-joined (2; slave/0) hook), holding 1s
unit-resourcemanager-0:
  holder: none
  waiting:
  - uniter (run relation-joined (1; namenode/0) hook), waiting 1s
unit-rsyslog-forwarder-ha-7:
  holder: none
  waiting:
  - uniter (run install hook), waiting 1s
unit-rsyslog-forwarder-ha-8:
  holder: none
```

When everything is idle, the output looks like this:

```text
machine-0:
  holder: none
unit-ganglia-node-7:
  holder: none
unit-ganglia-node-8:
  holder: none
unit-namenode-0:
  holder: none
unit-resourcemanager-0:
  holder: none
unit-rsyslog-forwarder-ha-7:
  holder: none
unit-rsyslog-forwarder-ha-8:
  holder: none
```





<!--TODO: Incorporate the below:

(logfile-varlogjujumachine-locklog)=
# Logfile: /var/log/juju/machine-lock.log

> See also: {ref}`Agent introspection: juju_machine_lock <agent-introspection-juju_machine_lock>`

A new log file was introduced in 2.3.9 and 2.4.2. The purpose of this log file is to give more visibility to who has been holding the machine lock.

The machine lock is used to serialize a number of activities of the agents on the machines started by Juju.

The machine agent will acquire the lock when it needs to install software to create containers, and also in some other instances.

The unit agents acquire the machine lock whenever they are going to execute hooks or run actions. Sometimes when there are multiple units on a given machine it is not always clear as to why something isn't happening as soon as you'd normally expect. This log file is to help give you insight into the actions of the agents.

This sample of output was taken from machine 0 in a hadoop-kafka deployment:

```text
2018-08-01 23:08:28 === agent unit-namenode-0 started ===
2018-08-01 23:08:30 unit-namenode-0: meterstatus (meter-status-changed), waited 0s, held 0s
2018-08-01 23:15:50 unit-namenode-0: uniter (run install hook), waited 0s, held 7m20s
2018-08-01 23:08:29 === agent unit-resourcemanager-0 started ===
2018-08-01 23:16:12 unit-resourcemanager-0: uniter (run install hook), waited 7m19s, held 22s
2018-08-01 23:16:14 unit-namenode-0: uniter (run leader-elected hook), waited 22s, held 2s
2018-08-01 23:16:16 unit-resourcemanager-0: uniter (run leader-elected hook), waited 2s, held 2s
2018-08-01 23:16:17 unit-namenode-0: uniter (run config-changed hook), waited 2s, held 2s
2018-08-01 23:16:19 unit-resourcemanager-0: uniter (run config-changed hook), waited 2s, held 2s
2018-08-01 23:16:21 unit-namenode-0: uniter (run start hook), waited 2s, held 2s
2018-08-01 23:16:23 unit-resourcemanager-0: uniter (run start hook), waited 2s, held 2s
2018-08-01 23:16:26 unit-namenode-0: uniter (run relation-joined (2; slave/0) hook), waited 2s, held 3s
2018-08-01 23:16:28 unit-resourcemanager-0: uniter (run relation-joined (1; namenode/0) hook), waited 3s, held 2s
2018-08-01 23:16:22 === agent unit-rsyslog-forwarder-ha-7 started ===
2018-08-01 23:16:38 unit-rsyslog-forwarder-ha-7: uniter (run install hook), waited 4s, held 10s
2018-08-01 23:16:22 === agent unit-ganglia-node-7 started ===
2018-08-01 23:16:43 unit-ganglia-node-7: uniter (run install hook), waited 13s, held 5s
2018-08-01 23:16:24 === agent unit-ganglia-node-8 started ===
2018-08-01 23:16:45 unit-ganglia-node-8: uniter (run install hook), waited 17s, held 2s
2018-08-01 23:16:47 unit-resourcemanager-0: uniter (run relation-changed (1; namenode/0) hook), waited 17s, held 2s
2018-08-01 23:16:50 unit-namenode-0: uniter (run relation-joined (1; resourcemanager/0) hook), waited 21s, held 3s
2018-08-01 23:16:52 unit-resourcemanager-0: uniter (run relation-joined (3; slave/0) hook), waited 4s, held 2s
2018-08-01 23:16:24 === agent unit-rsyslog-forwarder-ha-8 started ===
2018-08-01 23:16:54 unit-rsyslog-forwarder-ha-8: uniter (run install hook), waited 27s, held 2s
2018-08-01 23:16:54 unit-rsyslog-forwarder-ha-7: uniter (run leader-settings-changed hook), waited 17s, held 0s
2018-08-01 23:16:55 unit-ganglia-node-7: uniter (run leader-settings-changed hook), waited 12s, held 0s
2018-08-01 23:16:55 unit-ganglia-node-8: uniter (run leader-settings-changed hook), waited 10s, held 0s
2018-08-01 23:18:20 unit-resourcemanager-0: uniter (run relation-changed (1; namenode/0) hook), waited 3s, held 1m25s
2018-08-01 23:18:23 unit-namenode-0: uniter (run relation-changed (1; resourcemanager/0) hook), waited 1m30s, held 3s
2018-08-01 23:18:25 unit-ganglia-node-7: uniter (run config-changed hook), waited 1m29s, held 2s
2018-08-01 23:18:27 unit-ganglia-node-8: uniter (run config-changed hook), waited 1m30s, held 2s
2018-08-01 23:18:29 unit-ganglia-node-7: uniter (run start hook), waited 2s, held 1s
2018-08-01 23:18:29 unit-rsyslog-forwarder-ha-7: uniter (run config-changed hook), waited 1m35s, held 0s
2018-08-01 23:18:29 unit-rsyslog-forwarder-ha-8: uniter (run leader-settings-changed hook), waited 1m35s, held 0s
2018-08-01 23:18:30 unit-rsyslog-forwarder-ha-8: uniter (run config-changed hook), waited 0s, held 0s
2018-08-01 23:18:32 unit-resourcemanager-0: uniter (run relation-changed (3; slave/0) hook), waited 10s, held 2s
2018-08-01 23:18:34 unit-ganglia-node-8: uniter (run start hook), waited 5s, held 1s
2018-08-01 23:18:38 unit-namenode-0: uniter (run relation-joined (4; plugin/0) hook), waited 10s, held 4s
2018-08-01 23:18:39 unit-ganglia-node-7: uniter (run relation-joined (11; namenode/0) hook), waited 9s, held 1s
2018-08-01 23:18:39 unit-rsyslog-forwarder-ha-7: uniter (run start hook), waited 10s, held 0s
2018-08-01 23:18:39 unit-rsyslog-forwarder-ha-8: uniter (run start hook), waited 10s, held 0s
2018-08-01 23:18:43 unit-resourcemanager-0: uniter (run relation-joined (5; plugin/0) hook), waited 8s, held 3s
2018-08-01 23:18:45 unit-ganglia-node-8: uniter (run relation-joined (12; resourcemanager/0) hook), waited 10s, held 2s
2018-08-01 23:18:49 unit-namenode-0: uniter (run relation-changed (2; slave/0) hook), waited 8s, held 4s
2018-08-01 23:18:49 unit-ganglia-node-7: uniter (run relation-changed (11; namenode/0) hook), waited 10s, held 0s
2018-08-01 23:18:49 unit-rsyslog-forwarder-ha-7: uniter (run relation-joined (22; rsyslog/0) hook), waited 10s, held 0s
2018-08-01 23:18:50 unit-rsyslog-forwarder-ha-8: uniter (run relation-joined (22; rsyslog/0) hook), waited 10s, held 0s
2018-08-01 23:18:50 unit-ganglia-node-8: uniter (run relation-changed (12; resourcemanager/0) hook), waited 5s, held 0s
2018-08-01 23:18:50 unit-rsyslog-forwarder-ha-8: uniter (run relation-changed (22; rsyslog/0) hook), waited 0s, held 0s
2018-08-01 23:18:52 unit-ganglia-node-8: uniter (run relation-joined (16; ganglia/0) hook), waited 0s, held 1s
2018-08-01 23:18:53 unit-ganglia-node-7: uniter (run relation-joined (16; ganglia/0) hook), waited 3s, held 1s
2018-08-01 23:18:57 unit-resourcemanager-0: uniter (run relation-joined (3; slave/1) hook), waited 10s, held 4s
2018-08-01 23:18:57 unit-rsyslog-forwarder-ha-7: uniter (run relation-changed (22; rsyslog/0) hook), waited 7s, held 0s
2018-08-01 23:19:00 unit-namenode-0: uniter (run relation-changed (4; plugin/0) hook), waited 8s, held 4s
2018-08-01 23:19:04 unit-resourcemanager-0: uniter (run relation-changed (3; slave/1) hook), waited 4s, held 3s
2018-08-01 23:19:04 unit-rsyslog-forwarder-ha-7: uniter (run relation-joined (17; namenode/0) hook), waited 7s, held 0s
2018-08-01 23:19:04 unit-rsyslog-forwarder-ha-8: uniter (run relation-joined (18; resourcemanager/0) hook), waited 14s, held 0s
2018-08-01 23:19:09 unit-namenode-0: uniter (run relation-joined (4; plugin/1) hook), waited 4s, held 4s
2018-08-01 23:19:10 unit-ganglia-node-8: uniter (run relation-changed (16; ganglia/0) hook), waited 17s, held 1s
2018-08-01 23:19:10 unit-rsyslog-forwarder-ha-8: uniter (run relation-changed (18; resourcemanager/0) hook), waited 6s, held 0s
2018-08-01 23:19:12 unit-ganglia-node-7: uniter (run relation-changed (16; ganglia/0) hook), waited 18s, held 1s
2018-08-01 23:19:12 unit-rsyslog-forwarder-ha-7: uniter (run relation-changed (17; namenode/0) hook), waited 8s, held 0s
2018-08-01 23:19:16 unit-resourcemanager-0: uniter (run relation-joined (3; slave/2) hook), waited 9s, held 4s
2018-08-01 23:19:21 unit-namenode-0: uniter (run relation-joined (2; slave/1) hook), waited 8s, held 4s
2018-08-01 23:19:25 unit-resourcemanager-0: uniter (run relation-changed (3; slave/2) hook), waited 5s, held 4s
2018-08-01 23:19:29 unit-namenode-0: uniter (run relation-changed (2; slave/1) hook), waited 4s, held 4s
2018-08-01 23:19:33 unit-resourcemanager-0: uniter (run relation-changed (5; plugin/0) hook), waited 5s, held 4s
2018-08-01 23:19:38 unit-namenode-0: uniter (run relation-joined (2; slave/2) hook), waited 4s, held 5s
2018-08-01 23:19:42 unit-resourcemanager-0: uniter (run relation-changed (1; namenode/0) hook), waited 5s, held 4s
2018-08-01 23:19:47 unit-namenode-0: uniter (run relation-changed (2; slave/2) hook), waited 4s, held 5s
2018-08-01 23:19:51 unit-resourcemanager-0: uniter (run relation-joined (5; plugin/1) hook), waited 5s, held 4s
2018-08-01 23:19:56 unit-namenode-0: uniter (run relation-changed (4; plugin/1) hook), waited 5s, held 5s
2018-08-01 23:20:01 unit-resourcemanager-0: uniter (run relation-changed (1; namenode/0) hook), waited 5s, held 4s
2018-08-01 23:20:05 unit-resourcemanager-0: uniter (run relation-changed (5; plugin/1) hook), waited 0s, held 4s
2018-08-01 23:20:05 unit-namenode-0: meterstatus (meter-status-changed), waited 4s, held 0s
2018-08-01 23:20:05 unit-resourcemanager-0: meterstatus (meter-status-changed), waited 4s, held 0s
2018-08-01 23:20:32 unit-rsyslog-forwarder-ha-7: uniter (run update-status hook), waited 0s, held 0s
2018-08-01 23:20:52 unit-ganglia-node-8: uniter (run update-status hook), waited 0s, held 2s
2018-08-01 23:20:52 unit-rsyslog-forwarder-ha-8: uniter (run update-status hook), waited 2s, held 0s
2018-08-01 23:21:19 unit-ganglia-node-7: uniter (run update-status hook), waited 0s, held 2s
2018-08-01 23:22:28 unit-namenode-0: uniter (run update-status hook), waited 0s, held 8s
2018-08-01 23:22:32 unit-resourcemanager-0: uniter (run update-status hook), waited 7s, held 4s
```

There are a number of points of interest here to point out.

The times that the agents are started is recorded and written out to the file, but they are not actually written out to the log file until the agent writes its first entry. Each of the entries is written just before the release of the machine lock. You can see below here that the **`unit-resourcemanager-0`** agent started just one second after the **`unit-namenode-0`** agent, but the output of the line doesn't appear in time order. This is due to there being multiple processes wanting to write to a single file, so the file is only written to while the machine lock is held, and we don't want to stop an agent starting by waiting to acquire the lock just to write out that the agent has started.

```text
2018-08-01 23:08:28 === agent unit-namenode-0 started ===
2018-08-01 23:08:30 unit-namenode-0: meterstatus (meter-status-changed), waited 0s, held 0s
2018-08-01 23:15:50 unit-namenode-0: uniter (run install hook), waited 0s, held 7m20s
2018-08-01 23:08:29 === agent unit-resourcemanager-0 started ===
2018-08-01 23:16:12 unit-resourcemanager-0: uniter (run install hook), waited 7m19s, held 22s
```

Additionally normal line includes:
* a timestamp in UTC
* the agent name
* the worker inside that agent, and what it is acquiring the hook for
* how long the worker waited for the lock to be acquired
* how long the lock was held for

-->
