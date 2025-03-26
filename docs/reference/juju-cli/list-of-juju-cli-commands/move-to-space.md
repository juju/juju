(command-juju-move-to-space)=
# `juju move-to-space`
> See also: [add-space](#add-space), [spaces](#spaces), [reload-spaces](#reload-spaces), [rename-space](#rename-space), [show-space](#show-space), [remove-space](#remove-space)

## Summary
Update a network space's CIDR.

## Usage
```juju move-to-space [options] [--format yaml|json] <name> <CIDR1> [ <CIDR2> ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--force` | false | Allow to force a move of subnets to a space even if they are in use on another machine. |
| `--format` | human | Specify output format (human&#x7c;json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Examples

Move a list of CIDRs from their space to a new space:

	juju move-to-space db-space 172.31.1.0/28 172.31.16.0/20


## Details
Replaces the list of associated subnets of the space. Since subnets
can only be part of a single space, all specified subnets (using their
CIDRs) "leave" their current space and "enter" the one we're updating.