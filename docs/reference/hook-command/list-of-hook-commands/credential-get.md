(hook-command-credential-get)=
# `credential-get`
## Summary
Returns cloud connection details for a trusted application.

## Usage
```text
credential-get [options]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

Read the cloud connection details as JSON from a trusted charm hook:

    credential-get --format=json


## Details

Returns cloud connection details to a unit of a trusted application.
In a machine model, the result includes the model's credential when one
is configured. In a Kubernetes model, it includes the in-cluster API
endpoint and CA certificate, plus the unit's service-account token as
an OAuth2 credential. Keep output that contains credentials private.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/credential/