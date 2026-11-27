(command-juju-show-offer)=
# `juju show-offer`
> See also: [find-offers](#command-juju-find-offers)

## Summary
Show an offer's endpoints, description and access.

## Usage
```text
juju show-offer [options] <offer-name-or-url>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju show-offer hosted-mysql
    juju show-offer staging/default.hosted-mysql
    juju show-offer --format yaml mycontroller:staging/default.hosted-mysql


## Details

Give an offer URL without an endpoint, or just the offer name for an offer
in the selected model. The table shows the offer's description, access
and endpoints. Use YAML or JSON for the full description and the user
access entries visible to you.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/offer/