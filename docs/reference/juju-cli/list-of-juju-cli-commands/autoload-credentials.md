(command-juju-autoload-credentials)=
# `juju autoload-credentials`
> See also: [add-credential](#command-juju-add-credential), [credentials](#command-juju-credentials), [default-credential](#command-juju-default-credential), [remove-credential](#command-juju-remove-credential)

## Summary
Discovers cloud credentials and prompts you to save or upload them.

## Usage
```text
juju autoload-credentials [options] [<cloud-type>]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |

## Examples

Discover credentials interactively:

    juju autoload-credentials

Store detected credentials on this client:

    juju autoload-credentials --client

Upload detected credentials to a controller:

    juju autoload-credentials --controller mycontroller

Save selected credentials on this client and upload them to mycontroller:

    juju autoload-credentials --client --controller mycontroller

Search only the EC2 provider, then choose its cloud:

    juju autoload-credentials ec2


## Details

Search the registered providers that support credential discovery. To
search just one provider, supply its type, such as `ec2`. Select a
detected credential by number and choose a compatible cloud. An existing
credential with the same name can be replaced.

Use `--client` to save selections as you make them. Use
`--controller NAME` to upload selections for clouds available on
that controller when you finish by entering `q`. Check the result
for each credential before retrying.

Common discovery sources include:

- EC2
  - Credentials and regions:
    - `$HOME/.aws/credentials` and `$HOME/.aws/config` for the default region.
    - Environment variables: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`.

- GCE
  - Credentials:
    - A JSON file whose path is specified by the
       `GOOGLE_APPLICATION_CREDENTIALS` environment variable.
    - `$HOME/.config/gcloud/application_default_credentials.json`
      under the user's home directory.
    - Default region: `CLOUDSDK_COMPUTE_REGION`.

- OpenStack
  - Credentials:
    - `$HOME/.novarc`.
    - OpenStack environment variables, including `OS_USERNAME`,
      `OS_PASSWORD`, `OS_PROJECT_NAME` and `OS_TRUST_ID`.

- LXD
  - Credentials:
    - The local LXD socket and LXC configuration, including `$HOME/.config/lxc/config.yml`

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/credential/