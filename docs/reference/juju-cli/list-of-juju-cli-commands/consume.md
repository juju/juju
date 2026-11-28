(command-juju-consume)=
# `juju consume`
> See also: [integrate](#command-juju-integrate), [offer](#command-juju-offer), [remove-saas](#command-juju-remove-saas)

## Summary
Use a remote offer as a SAAS application in the consuming model.

## Usage
```text
juju consume [options] <offer-url> [<local-application-name>]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Consume the `mysql` offer from `othermodel` in the current model:

    juju consume othermodel.mysql

Consume the `mysql` offer from `prod/othermodel` on `anothercontroller` in model `frontend` under the local name `orders-db`:

    juju consume -m frontend anothercontroller:prod/othermodel.mysql orders-db


## Details
The model selected with `-m` is the consuming model. Consuming an offer
does not create a relation. To create one, use `juju integrate`.

The path to the remote offer is formatted as follows:

    [<controller name>:][<model qualifier>/]<model name>.<offer name>

If you omit the controller, Juju looks for the offer on the consuming
model's controller. If you omit the model qualifier, Juju uses your user
name on that controller, even when the offer is on another controller.

The optional local name defaults to the offer name. The command prints
the name it requested, not the name it found. If the model already
consumed the offer, check `juju status` for the name in use.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/offer/