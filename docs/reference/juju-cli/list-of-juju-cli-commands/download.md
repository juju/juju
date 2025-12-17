(command-juju-download)=
# `juju download`
> See also: [info](#info), [find](#find)

## Summary
Locates and then downloads a Charmhub charm.

## Usage
```juju download [options] [options] <charm>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--arch` | all | Specifies an arch &lt;all&#x7c;amd64&#x7c;arm64&#x7c;ppc64el&#x7c;riscv64&#x7c;s390x&gt;. |
| `--base` |  | Specifies a base. |
| `--channel` |  | Specifies a channel to use instead of the default release. |
| `--charmhub-url` | https://api.charmhub.io | Specifies the Charmhub URL for querying the store. |
| `--filepath` |  | Specifies the filepath location of the charm to download to. |
| `--no-progress` | false | Specifies whether to disable the progress bar. |
| `--resources` | false | (DEPRECATED IN JUJU 4.0 BECAUSE DEFAULT BEHAVIOR) Specifies whether to download the resources associated with the charm (will be default behaviour in 4.0). |
| `--revision` | -1 | Specifies a revision of the charm to download. |
| `--series` | all | (DEPRECATED) Specifies a series. Deprecated: use `--base`. |

## Examples

    juju download postgresql
    juju download postgresql --no-progress - > postgresql.charm


## Details

Downloads a charm to the current directory from the Charmhub store
by a specified name. Downloading for a specific base can be done via
`--base`. `--base` can be specified using the OS name and the version of
the OS, separated by `@`. For example, `--base ubuntu@22.04`.

By default, the latest revision in the default channel will be
downloaded. To download the latest revision from another channel,
use `--channel`. To download a specific revision, use `--revision`,
which cannot be used together with `--arch`, `--base`, `--channel` or
`--series`.

Adding a hyphen as the second argument allows the download to be piped
to `stdout`.