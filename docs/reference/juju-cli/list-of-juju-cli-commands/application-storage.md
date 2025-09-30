(command-juju-application-storage)=
# `juju application-storage`
> See also: [storage](#storage), [storage-pools](#storage-pools), [add-unit](#add-unit)

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

Set the size to 100GiB, pool name to "rootfs", and count to 1 for the mysql application's 'database' storage:

    juju application-storage mysql database=100G,rootfs,1

If no size is provided, Juju uses the minimum size required by the charm. If the charm does not specify a minimum, the default is 1 GiB. 
This value is then applied when updating the application’s storage.

    juju application-storage mysql database=,rootfs,1

If no pool is provided, Juju selects the default storage pool from the model.
This pool will be recorded as the updated value for the application’s storage.

	juju application-storage mysql database=100G,,1

If no count is provided, Juju uses the minimum count required by the charm. 
That count will be used when updating the application’s storage.

	juju application-storage mysql database=100G,rootfs,

To set a storage directives for an application from a file:

    juju application-storage mysql --file=path/to/cfg.yaml

Note: The order of size, pool, and count in the assignment does not matter.
For example, the following are equivalent:

    juju application-storage mysql database=100G,rootfs,1
    juju application-storage mysql database=rootfs,1,100G


## Details

To view all storage directives values for the given application:

    juju application-storage <application>

	By default, the config will be printed in a tabular format. You can instead
print it in `json` or `yaml` format using the `--format` flag:

   	juju application-storage &lt;application&gt; --format json
    juju application-storage <application> --format yaml

To view the value of a single storage name:

    juju application-storage <application> <storage-name>

To set storage constraint values on an application:

    juju application-storage <application> name1=size, name2=pool, name3=count

Config values can be imported from a yaml file using the `--file` flag:

    juju application-storage <application> --file=path/to/constraints.yaml

This allows you to, e.g., save an application's storage directives to a file:

    juju application-storage <application> --format=yaml > constraints.yaml

and then import the config later.