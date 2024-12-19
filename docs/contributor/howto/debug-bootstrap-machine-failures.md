# Debug bootstrap machine failures

This guide will show you how to diagnose and fix issues with bootstrapping and starting new machines.

Juju's bootstrapping process can be broken down into several steps:

1. Provision resources/a machine `M` from the relevant cloud

2. Install the Juju agent `jujud` on machine `M`

3. Poll the newly created instance for an IP address, and attempt to connect to `M`

4. Run the machine configuration script for `M`, which e.g. installs relevant packages

The output of `juju bootstrap` will tell you which step you're at. If your failure is at step 1, then the issue is most
likely with your cloud provider or configuration - see the guides [here](https://juju.is/docs/olm/manage-clouds).

Otherwise, we need to connect to the machine and look at the logs to find out what's gone wrong.

<!-- TOC -->

* [Connect to the machine](#connect-to-the-machine)
    * [Via ssh](#via-ssh)
    * [Via the cloud provider](#via-the-cloud-provider)
        * [LXC / LXD](#lxc--lxd)
        * [Kubernetes](#kubernetes)
* [Examine the logs](#examine-the-logs)

<!-- TOC -->

# Connect to the machine

## Via ssh

The easiest way to connect to the machine is via ssh. We can do this ***if*** Juju has been successfully able to connect
to your controller. In this case, you will see the line

```
Connected to [ip-address]
```

in your `juju bootstrap` output.

A common type of failure here is when the terminal hangs on the line

```
Running machine configuration script...
```

The machine configuration should take less than 10 minutes - any longer than this is a sign that something has gone
wrong.

Luckily, the machine is already reachable at this step, so we can directly `ssh` into it to find out what's happening.
Copy the IP address that Juju connected to above, and run

```
ssh ubuntu@[ip-address] -i [juju-data-dir]/ssh/juju_id_rsa
```

Here, `[juju-data-dir]` defaults to `~/.local/share/juju`, but if you've set the `JUJU_DATA` environment variable, it
will be equal to that instead.

See [here](https://juju.is/docs/olm/accessing-individual-machines-with-ssh) for a more in-depth guide on using SSH to
connect to a machine.

## Via the cloud provider

If Juju wasn't able to connect to your machine's IP address, then `ssh` probably won't be able to either. With this type
of failure, you'll often see your terminal hang after the step

```
Attempting to connect to [ip-address]:[port]
```

In this case, we will need to go through the cloud provider to connect to the machine. The process here depends on what
cloud you're using.

### LXC / LXD

In the `juju bootstrap` output, you should see a line like

```
Launching controller instance(s) on localhost/localhost...
```

which will be followed by the LXD container name (in the form `juju-XXXXXX-0`). We can use the `lxc` command line tool
to get a shell inside the machine. Copy the container name, then run

```
lxc exec [container-name] bash
```

Now, we should have a shell inside the machine, and can use the steps below to search the logs.

### Kubernetes

In the `juju bootstrap` output, you should see a line like

```
Creating k8s resources for controller [namespace]
```

where `[namespace]` is something like `controller-foobar`. Inside this namespace, Juju will have created a pod called
`controller-0` - we want to access the `api-server` container in this pod. To do this, we use `kubectl`:

```
kubectl exec controller-0 -itc api-server -n [namespace] -- bash
```

(If using MicroK8s, call this command via `microk8s kubectl`).

# Examine the logs

Once we have a shell inside the machine, we can

```
ls /var/log
```

which will show you all the available logs. Which log to look at depends on the type of failure, but generally speaking,
`syslog`, `cloud-init.log` and `cloud-init-output.log` are good ones to look at.

Some good tools for examining logs are

```
less [log-file]
```

which will let you scroll through the log file, and

```
tail -f [log-file]
```

which will track updates to the log file.

Errors (especially fatal ones) will often be near the end of a log file. You may also have luck searching your logs for
phrases such as "error" or "fail".

<!--stackedit_data:

eyJoaXN0b3J5IjpbMTU3NjQ2MDc3NCwzODI3NDgzLC04NzA4Nj

E0MTQsLTE5Nzc1Mzg3NzgsLTg5MTkxOTAyOSw2NDE0NDk4ODUs

Njg0MTczMzIzLC0xODUyMjIxNDgxLDE4NjMzNjU2MTIsMTk1MD

YwMjgyNSwtMTAwMTQ3NzExNSwtNzI1MzE0OTc3LC0xNzQ0MTcx

MzM5LC0xMjgzMzQyOTIsMTUyMjkyMDYwLDE5NzkxODUxNzEsLT

E2MjQ2MTExNzUsOTc3OTYwNDY0LDczMDk5ODExNl19

-->