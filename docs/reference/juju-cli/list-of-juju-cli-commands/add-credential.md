(command-juju-add-credential)=
# `juju add-credential`
> See also: [credentials](#command-juju-credentials), [remove-credential](#command-juju-remove-credential), [update-credential](#command-juju-update-credential), [default-credential](#command-juju-default-credential), [default-region](#command-juju-default-region), [autoload-credentials](#command-juju-autoload-credentials)

## Summary
Adds cloud credentials to this client, a controller, or both.

## Usage
```text
juju add-credential [options] <cloud name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |
| `-f`, `--file` |  | The YAML file containing credentials to add |
| `--region` |  | Cloud region to use when preparing the credential |

## Examples

Enter a Google Cloud credential and save it on this client:

    juju add-credential google --client

Enter a Google Cloud credential and upload it to mycontroller:

    juju add-credential google --controller mycontroller

Import the AWS entries from a YAML file to this client:

    juju add-credential aws --file credentials.yaml --client

Import the AWS entries from a YAML file and upload them to mycontroller:

    juju add-credential aws --file credentials.yaml --controller mycontroller


## Details

Run `juju add-credential CLOUD` to enter material at prompts.
Use `--file` to read credential material for the named cloud from YAML.
For an unattended file import, also specify `--client`,
`--controller NAME`, or both. Use `--region` to select
the region for credential preparation.

A credentials file maps cloud names to named entries, each with an
authentication type and its provider-specific attributes. For example,
if you have an EC2 instance profile named juju-production:

    credentials:
      aws:
        production-deploy:
          auth-type: instance-role
          instance-profile-name: juju-production

Names must be unique within a cloud in the file. File import skips an
existing local name. If you also select a controller, uploading the
entry can replace shared material under that name. Use
`juju update-credential` to replace a local entry deliberately.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/credential/