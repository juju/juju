(hook-command-is-leader)=
# `is-leader`
## Summary
Print application leadership status.

## Usage
``` is-leader [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    LEADER=$(is-leader)
    if [ "${LEADER}" == "True" ]; then
      # Do something a leader would do
    fi


## Details

is-leader prints a boolean indicating whether the local unit is guaranteed to
be application leader for at least 30 seconds. If it fails, you should assume that
there is no such guarantee.