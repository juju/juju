(command-juju-offers)=
# `juju offers`
> See also: [find-offers](#command-juju-find-offers), [show-offer](#command-juju-show-offer)

**Aliases:** list-offers

## Summary
List offers in a model and their connections.

## Usage
```text
juju offers [options] [<offer-name>]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--active-only` | false | Show only offers with at least one connection, even if none are joined |
| `--allowed-consumer` |  | Match offers on which the user has a consume or admin grant |
| `--application` |  | Return results matching the application |
| `--connected-user` |  | Not applied by a Juju 4.1 controller; see the User column |
| `--format` | tabular | Specify output format (json&#x7c;summary&#x7c;tabular&#x7c;yaml) |
| `--interface` |  | Return results matching the interface name |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Examples

List offers and their connections in the current model:

    juju offers

List offers and their connections in the model named `model`:

    juju offers -m model

List offers with an endpoint that uses the `db2` interface:

    juju offers --interface db2

List offers for endpoints of the `mysql` application:

    juju offers --application mysql

Show a summary with one row per offered endpoint and connection counts for its offer:

    juju offers --format summary

List offers on which `mary` has a consume or admin grant:

    juju offers --allowed-consumer mary

List offers whose names contain `hosted-mysql`:

    juju offers hosted-mysql

List offers whose names contain `hosted-mysql` and that have at least one connection:

    juju offers hosted-mysql --active-only


## Details

List offers in the selected model. The optional name matches any offer name
containing that text. To inspect one exact offer, use `juju show-offer`.

The default table shows one row per connection, with its user, relation ID
and status. An offer without connections has a dash in the User column.
In `--format summary`, each endpoint has a row. Connected shows joined
connections over total connections for the offer, not for the endpoint.
YAML and JSON also include the consuming model UUID for each connection.

To check which offers a user is connected to, read the User column in
the default table. To check an offer's grants, use `--format yaml`.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/offer/