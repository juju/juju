(command-juju-info)=
# `juju info`
> See also: [find](#find), [download](#download)

## Summary
Displays detailed information about CharmHub charms.

## Usage
```juju info [options] [options] <charm>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--arch` | all | Specify an arch &lt;all&#x7c;amd64&#x7c;arm64&#x7c;ppc64el&#x7c;riscv64&#x7c;s390x&gt; |
| `--base` |  | Specify a base |
| `--channel` |  | Specify a channel to use instead of the default release |
| `--charmhub-url` | https://api.charmhub.io | Specify the Charmhub URL for querying the store |
| `--config` | false | Display config for this charm |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--revision` | -1 | Specify a revision number |
| `--series` | all | Specify a series. DEPRECATED use --base |
| `--track` |  | Specify a track to use instead of the default track |
| `--unicode` | auto | Display output using unicode &lt;auto&#x7c;never&#x7c;always&gt; |

## Examples

    juju info postgresql


## Details

The charm can be specified by name or by path.

Channels displayed are supported by any base.
To see channels supported for only a specific base, use the --base flag.
--base can be specified using the OS name and the version of the OS, 
separated by @.
For example: --base ubuntu@22.04

Use --revision to display information about a specific revision of the charm,
which cannot be used together with --arch, --base, --channel or --series.
For example: --revision 42

Use --track to display information about a specific track of the charm,
which cannot be used together with --arch, --base, --channel or --series.
For example: --track 14