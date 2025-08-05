(command-juju-list-offers)=
# `juju list-offers`
> See also: [find-offers](#find-offers), [show-offer](#show-offer)

**Aliases:** list-offers

## Summary
Lists shared endpoints.

## Usage
```juju offers [options] [<offer-name>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--active-only` | false | only return results where the offer is in use |
| `--allowed-consumer` |  | return results where the user is allowed to consume the offer |
| `--application` |  | return results matching the application |
| `--connected-user` |  | return results where the user has a connection to the offer |
| `--format` | tabular | Specify output format (json&#x7c;summary&#x7c;tabular&#x7c;yaml) |
| `--interface` |  | return results matching the interface name |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju offers
    juju offers -m model
    juju offers --interface db2
    juju offers --application mysql
    juju offers --connected-user fred
    juju offers --allowed-consumer mary
    juju offers hosted-mysql
    juju offers hosted-mysql --active-only


## Details

List information about applications' endpoints that have been shared and who is connected.

The default tabular output shows each user connected (relating to) the offer, and the 
relation id of the relation.

The summary output shows one row per offer, with a count of active/total relations.

The YAML output shows additional information about the source of connections, including
the source model UUID.

The output can be filtered by:
 - interface: the interface name of the endpoint
 - application: the name of the offered application
 - connected user: the name of a user who has a relation to the offer
 - allowed consumer: the name of a user allowed to consume the offer
 - active only: only show offers which are in use (are related to)