(command-juju-enable-ha)=
# `juju enable-ha`
## Summary
Ensures that sufficient controllers exist to provide redundancy.

## Usage
```juju enable-ha [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-c`, `--controller` |  | Specifies the controller to operate in. |
| `--constraints` | [] | Specifies additional machine constraints. |
| `--format` | simple | Specify output format (json&#x7c;simple&#x7c;yaml) |
| `-n` | 0 | Specifies the number of controllers to make available. |
| `-o`, `--output` |  | Specify an output file |
| `--to` |  | Specifies the machine(s) to become controllers, bypassing constraints. |

## Examples

Ensure that the controller is still in highly available mode. If there is only 1 controller running, this will ensure there
are 3 running. If you have previously requested more than 3,
then that number will be ensured.

    juju enable-ha

Ensure that 5 controllers are available:

    juju enable-ha -n 5

Ensure that 7 controllers are available, with newly created
controller machines having at least 8GB RAM.

    juju enable-ha -n 7 --constraints mem=8G

Ensure that 7 controllers are available, with machines `server1` and
`server2` used first, and if necessary, newly created controller
machines having at least 8GB RAM.

    juju enable-ha -n 7 --to server1,server2 --constraints mem=8G


## Details

To ensure availability of deployed applications, the Juju infrastructure
must itself be highly available. The `enable-ha` command ensures
that the specified number of controller machines are used to make up the
controller.

An odd number of controllers is required.