> See also: [info](#info), [download](#download)

## Summary
Queries the CharmHub store for available charms or bundles.

## Usage
```juju find [options] [options] <query>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--category` |  | filter by a category name |
| `--channel` |  | filter by channel" |
| `--charmhub-url` | https://api.charmhub.io | specify the Charmhub URL for querying the store |
| `--columns` | nbvps | display the columns associated with a find search.      The following columns are supported:         - n: Name         - b: Bundle         - v: Version         - p: Publisher         - s: Summary 		- a: Architecture 		- o: OS         - S: Supports  |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--publisher` |  | search by a given publisher |
| `--type` |  | search by a given type &lt;charm&#x7c;bundle&gt; |

## Examples

    juju find wordpress


## Details

The find command queries the CharmHub store for available charms or bundles.



