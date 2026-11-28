(command-juju-find-offers)=
# `juju find-offers`
> See also: [show-offer](#command-juju-show-offer)

## Summary
Find offers visible to you on a controller.

## Usage
```text
juju find-offers [options] [<offer-url-term>]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `--interface` |  | Return results matching the interface name |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |
| `--offer` |  | Match offer names containing this text |
| `--url` |  | Specify the URL term instead of a positional argument; cannot be used with --offer |

## Examples

Find offers visible to you on the current controller:

    juju find-offers

Find offers visible to you on the controller named `mycontroller`:

    juju find-offers mycontroller:

Find visible offers in model `staging/mymodel`:

    juju find-offers staging/mymodel

Find visible offers with an endpoint that uses the `mysql` interface:

    juju find-offers --interface mysql

Find visible offers whose names contain `db2` in model `staging/mymodel` on `mycontroller`:

    juju find-offers mycontroller:staging/mymodel.db2

Find visible offers whose names contain `db2` on the current controller:

    juju find-offers --offer db2



## Details

Search for offers you can see. Without a URL term, the search covers all
models on the selected controller, not just the current model. Use a
controller name followed by a colon to search another controller, or give
a model to limit the search to that model.

Use `--offer` without a URL term to search by part of an offer name.
With a positional URL term, its offer-name component replaces `--offer`,
even when that component is empty. To search by name in a particular
model, include the name in the URL term. If nothing matches, the command
returns an error.

The table lists interfaces as interface:endpoint pairs. Finding an offer
does not by itself mean you have permission to consume it.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/offer/