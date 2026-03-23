---
myst:
  html_meta:
    description: "Check hook execution lock status across Juju agents using juju_machine_lock to see which units hold or wait for locks."
---

(juju_machine_lock)=
# `juju_machine_lock`

This function actually calls into every agent on the machine to ask about the agent's view of the hook execution lock. Where the {ref}`machine-lock.log <the-machine-lock-log-file>` file shows the history of the machine lock, the introspection endpoint shows the current status of the lock, whether the agent holds the lock, or is waiting for the lock.

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

