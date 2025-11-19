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

Set the size to 10GiB, pool name to "rootfs", and count to 1 for the mysql application's 'database' storage specification:

    juju application-storage mysql database=10G,rootfs,1
	OR
    juju application-storage mysql database=rootfs,1,10G
	OR
    juju application-storage mysql database=1,10G,rootfs


## Details

A storage directive describes to the charm how to refer to the storage,
and where to provision it from and takes the form &lt;storage-name&gt;[=&lt;storage-specification&gt;]; for details
see https://documentation.ubuntu.com/juju/3.6/reference/storage/#storage-directive.

To view all storage directives for the given application:

    juju application-storage <application>

By default, the config will be printed in a tabular format. You can instead
print it in `json` or `yaml` format using the `--format` flag:

   	juju application-storage &lt;application&gt; --format json
    juju application-storage <application> --format yaml

To view the directive for a single storage name:

    juju application-storage <application> <storage-name>

To set storage directives for an application:

    juju application-storage <application> <storagename1>=<storage-specification> <storagename2>=<storage-specification> ...