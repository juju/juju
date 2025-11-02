(debug-a-dqlite-core-dump-issue)=
# Debug a Dqlite core dump issue

If you are on Juju 3.2+, and your `juju status` suggests the agent is lost, you may have a core dump issue. This document shows how to validate this suspicion and then get the backtrace so the issue can eventually be reproduced and addressed.


## Check if you do in fact have a core dump issue


Check the database logs:


```text
$ juju debug-log | grep "core dump"
```

Sometimes logs don’t make it to the database, so also check the controller machine logs:

```text
$ juju ssh -m controller <controller machine>
$ grep “core dump” /var/log/juju/$(juju_controller_agent_name).log
```

If the results show `(core dumped)`, you have a core dump issue.

````{dropdown} Example results that point to a core dump issue
```text
/etc/systemd/system/jujud-machine-0-exec-start.sh: line 11: 5862 Segmentation fault (core dumped) '/var/lib/juju/tools/machine-0/jujud' machine --data-dir '/var/lib/juju' --machine-id 0 --debug
```
or
```text
Assertion failed: type == SQLITE_INTEGER || type == SQLITE_NULL (src/query.c: value_type: 23)
signal: aborted (core dumped)
```
````


## Retrieve the core dump backtrace

1. Open juju/Makefile and, in [the line with `CGO_LINK_FLAGS`](https://github.com/juju/juju/blob/528c205d9995d3fb85ac041ad79495dff8ee4eda/Makefile#L225), remove the `-s` flag. This will ensure that the `jujud-controller` binary contains the debug symbols.

2. Bootstrap a controller with the modified binary. Once the controller is running, SSH into the controller machine (usually machine 0) and install the `gdb` package:

```bash
$ juju ssh -m controller 0
$ sudo apt install gdb
```

3. Stop the controller machine:

```bash
$ sudo systemctl stop jujud-machine-0.service
```

4. Start the controller with `gdb` and reproduce the crash:

```bash
$ LIBDQLITE_TRACE=1 gdb -ex=r --args /var/lib/juju/tools/machine-0/jujud machine --data-dir "/var/lib/juju" --machine-id 0 --debug
```

This will run the controller. You should keep it running until you reproduce the crash.

If you encounter SIGPIPE errors, which will stop the controller, ignore them by running the following command in the `gdb` prompt:

```
gdb> handle SIGPIPE nostop noprint pass
gdb> continue
```

Once the controller crashes, it should put you back into the gdb prompt. At this point, you can get the backtrace with the following command:

```
gdb> bt
```

Grab the output of the backtrace and share it with the Juju team. They will be able to help you diagnose the issue further.
