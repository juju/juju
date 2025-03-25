(hook-command-unit-get)=
# `unit-get`

## Summary
Print public-address or private-address.

## Usage
``` unit-get [options] <setting>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Details

Further details:
unit-get returns the IP address of the unit.

It accepts a single argument, which must be
private-address or public-address. It is not
affected by context.

Note that if a unit has been deployed with
--bind space then the address returned from
unit-get private-address will get the address
from this space, not the ‘default’ space.