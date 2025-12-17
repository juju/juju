(command-juju-find)=
# `juju find`
> See also: [info](#info), [download](#download)

## Summary
Queries the Charmhub store for available charms or bundles.

## Usage
```juju find [options] [options] <query>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--category` |  | Filters by a category name. |
| `--channel` |  | Filters by channel. |
| `--charmhub-url` | https://api.charmhub.io | Specifies the Charmhub URL for querying the store. |
| `--columns` | nbvps | Displays the columns associated with a find search.      The following columns are supported:         `n`: Name;         `b`: Bundle;         `v`: Version;         `p`: Publisher;         `s`: Summary; 		`a`: Architecture; 		`o`: OS;         `S`: Supports.  |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--publisher` |  | Searches by a given publisher. |
| `--type` |  | Searches by a given type  from &lt;charm&#x7c;bundle&gt;. |

## Examples

    juju find wordpress


## Details

Queries the Charmhub store for available charms or bundles.