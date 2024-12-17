(plugin-flags)=
# Plugin flags

Plugins are independent command-line scripts, so, like any executable, they can accept `--flags`. There is potential for confusion to arise when a plugin accepts a flag which has the same name as one of Juju's global options:

```text
--debug
-h, --help
--logging-config
--quiet
--show-log
--verbose
```

In general, we wouldn't recommend using a flag name from this list but, if you do, ensure you put the flag in the correct position. Global options intended for Juju should come **before** the plugin name:

```text
juju --debug myplugin foo
```
while flags intended for the plugin should come **after** the plugin name:

```text
juju myplugin --debug foo
juju myplugin foo --debug
```
