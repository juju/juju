(juju-dumplogs)=
# `juju-dumplogs`

> Only available on a controller machine / container

## Summary

Output the logs that are stored in the local Juju database.

## Usage

```text
juju-dumplogs [options]
```


### Options

```text
-d, --output-directory (= ".")
    directory to write logs files to
--data-dir (= "/var/lib/juju")
    directory for juju data
--machine-id (= "")
    id of the machine on this host (optional)
```

## Details
This tool can be used to access Juju's logs when the Juju controller
isn't functioning for some reason. It must be run on a Juju controller
server, connecting to the Juju database instance and generating a log
file for each model that exists in the controller.

Log files are written out to the current working directory by
default. Use -d / --output-directory option to specify an alternate
target directory.

In order to connect to the database, the local machine agent's
configuration is needed. In most circumstances the configuration will
be found automatically. The --data-dir and/or --machine-id options may
be required if the agent configuration can't be found automatically.


