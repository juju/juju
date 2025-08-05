> See also: [spaces](#spaces), [remove-space](#remove-space)

## Summary
Add a new network space.

## Usage
```juju add-space [options] <name> [<CIDR1> <CIDR2> ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples


Add space "beta" with subnet 172.31.0.0/20:
    
    juju add-space beta 172.31.0.0/20


## Details
Adds a new space with the given name and associates the given
(optional) list of existing subnet CIDRs with it.


