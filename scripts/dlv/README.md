# Debugging Juju

[Delve]: https://github.com/go-delve/delve

Juju is a distributed system, with many binaries running in many machine, which can be tricky to debug. This document
is aimed to help to understand how to run Juju in debug mode.

## Setup everything

To debug a jujud or jujud-controller, we will use [Delve]. To be able to access to codebase, [Delve] requires
that a binary has been compiled with specific option. For juju, it implies to set a specific environment variable. So, 
in order to be able to bootstrap a controller with debugging capabilities, we need to build with the following command:

```shell
DEBUG_JUJU=1 make install
```

`DEBUG_JUJU` will enable required flags and tags and embed [Delve] into the binary[^1]. 

[^1]: Check the [Makefile](../../Makefile), [dlv package](../../internal/dlv/doc.go) and maybe a 
[command with debug enabler](../../cmd/jujud-controller/main_debug.go)

## Debugging

Once build with debugging capabilities, we can debug Juju on our favorite IDE. 

### Juju command

To debug any juju command, just run the built command with [Delve]. It can be through `delve exec`, or even simply by 
using `go build` configuration in any good IDE. 

###  `jujud` / `jujud-controller`

Once built with delve, bootstrapping, juju will run `jujud` on the machine with a dedicated unix socket
to connect a [Delve] remote instance.
However, the unix socket is on the remote machine, and to access the socket from your local IDE requires
additional work. That is why we need to open an SSH tunnel to the socket, binding it with a local port on the 
local machine. It is the purpose of the [dlv/juju.sh script](juju.sh).

To launch a debug session linked to our current controller, just run the script without argument:

```shell
./scripts/dlv/juju.sh
```

It will then be possible to link a [Delve] remote session through `127.0.0.1:2345`. The script support several option to:

- run in detached mode
- specify the listening port
- specify an identity key 
- specify the controller, the model and/or the machine 
- specify the remote unix socket

## What is going under the hood

Behind the scene, `jujud` is running by a [Delve], listening to an unix socket named with the following pattern:

`/path/to/jujud.<subcommand>.socketd`

The controller machine will listen on `/var/lib/juju/tools/4.0-beta5.1-ubuntu-amd64/jujud.machine.socketd` with:

* `/path/to`: `/var/lib/juju/tools/4.0-beta5.1-ubuntu-amd64/`
* `subcommand`: `machine` (in that case, the controller agent has been run by `juju machine <args>`)

If there are multiple `jujud` processes running, you can choose one when running the script.

The script opens an ssh tunnel linking the socket with the local machine on the specified port.

## Troubleshooting

### When running the script, I get a "Permission denied (publickey)"

You are probably trying to run the debugger on a non-controller machine. Those machine aren't accessible by default 
through SSH. You will need to add a valid public key to the authorized key of the machine, for instance on LXD:

```shell
cat ~/.local/share/juju/ssh/juju_id_ed25519.pub > ~/tmp/authorized_keys
lxc file push ~/tmp/authorized_keys juju-79af58-2/home/ubuntu/.ssh/authorized_keys
```

Then, the script should work as expected.
