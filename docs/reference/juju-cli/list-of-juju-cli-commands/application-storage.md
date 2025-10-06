(command-juju-application-storage)=
# `juju application-storage`
> See also: [storage](#storage), [storage-pools](#storage-pools), [add-storage](#add-storage), [add-unit](#add-unit)

## Summary
Displays or sets storage directives for an application.

## Usage
```juju application-storage [options] <application-name> [<storage-name>[={<size>,<pool>,<count>}]] ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--color` | false | Use ANSI color codes in output |
| `--file` |  | Path to yaml-formatted configuration file |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-color` | false | Disable ANSI color codes in tabular output |
| `-o`, `--output` |  | Specify an output file |

## Examples

Print the storage directives for all storage names of the postgresql application:

    juju application-storage postgresql

Print the storage directives for the storage name 'pgdata' of the postgresql application:

    juju application-storage postgresql pgdata

Set the size to 10GiB, pool name to "rootfs", and count to 1 for the mysql application's 'database' storage:

    juju application-storage mysql database=10G,rootfs,1
	OR
    juju application-storage mysql database=rootfs,1,10G
	OR
    juju application-storage mysql database=1,10G,rootfs

If no size is provided, Juju uses the minimum size required by the charm. If the charm does not specify a minimum, the default is 1 GiB. 
This value is then applied when updating the application’s storage.

    juju application-storage mysql database=,rootfs,1

If no pool is provided, Juju selects the default storage pool from the model.
This pool will be recorded as the updated value for the application’s storage.

	juju application-storage mysql database=10G,,1

If no count is provided, Juju uses the minimum count required by the charm. 
That count will be used when updating the application’s storage.

	juju application-storage mysql database=10G,rootfs,


## Details

To view all storage directives for the given application:

    juju application-storage <application>

	By default, the config will be printed in a tabular format. You can instead
print it in `json` or `yaml` format using the `--format` flag:

   	juju application-storage &lt;application&gt; --format json
    juju application-storage <application> --format yaml

To view the directive of a single storage name:

    juju application-storage <application> <storage-name>

To set storage directives on an application:

    juju application-storage <application> <storagename1>=<storage-directive> <storagename2>=<storage-directive> ...

`<storage-directive>` describes to the charm how to refer to the storage,
and where to provision it from. `<storage-directive>` takes the following form:

    <storage-name>[=<storage-configuration>]

`<storage-name>` is defined in the charm's `metadata.yaml` file.

`<storage-configuration>` is a description of how Juju should provision storage
instances for the unit. They are made up of up to three parts: `<pool>`,
`<count>`, and `<size>`. They can be provided in any order, but we recommend the
following:

    <pool>,<count>,<size>

Each parameter is optional, so long as at least one is present. So the following
storage constraints are also valid:

    <pool>,<size>
    <count>,<size>
    <size>

`<pool>` is the storage pool to provision storage instances from. Must
be a name from `juju storage-pools`.  The default pool is available via
executing `juju model-config storage-default-block-source` or `storage-default-filesystem-source`.

`<count>` is the number of storage instances to provision from `<storage-pool>` of
`<size>`. Must be a positive integer. The default count is `1`. May be restricted
by the charm, which can specify a maximum number of storage instances per unit.

`<size>` is the number of bytes to provision per storage instance. Must be a
positive number, followed by a size suffix.  Valid suffixes include M, G, T,
and P.  Defaults to "1024M", or the which can specify a minimum size required
by the charm.