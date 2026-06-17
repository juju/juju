(command-juju-autoload-credentials)=
# `juju autoload-credentials`
> See also: [add-credential](#add-credential), [credentials](#credentials), [default-credential](#default-credential), [remove-credential](#remove-credential)

## Summary
Attempts to automatically detect and add credentials for a cloud.

## Usage
```juju autoload-credentials [options] [<cloud-type>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |

## Examples

    juju autoload-credentials
    juju autoload-credentials --client
    juju autoload-credentials --controller mycontroller
    juju autoload-credentials --client --controller mycontroller
    juju autoload-credentials aws


## Details

The command searches well known, cloud-specific locations on this client.
If credential information is found, it is presented to the user
in a series of prompts to facilitated interactive addition and upload.
An alternative to this command is `juju add-credential`.

After validating the contents, credentials are added to
this Juju client if `--client` is specified.

To upload credentials to a controller, use `--controller` option.

Below are the cloud types for which credentials may be autoloaded,
including the locations searched.

- EC2
  - Credentials and regions:
    - On Linux, `$HOME/.aws/credentials` and `$HOME/.aws/config`.
    - Environment variables: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`.

- GCE
  - Credentials:
    - A JSON file whose path is specified by the
       `GOOGLE_APPLICATION_CREDENTIALS` environment variable.
    - On Linux, `$HOME/.config/gcloud/application_default_credentials.json`.
       Default region is specified by the `CLOUDSDK_COMPUTE_REGION` environment
       variable.
    - On Windows, `%APPDATA%gcloudpplication_default_credentials.json`.

- OpenStack
  - Credentials:
    - On Linux, `$HOME/.novarc`.
    - Environment variables: `OS_USERNAME`, `OS_PASSWORD`, `OS_TENANT_NAME`,
	   `OS_DOMAIN_NAME`.

- LXD
  - Credentials:
    - On Linux, `$HOME/.config/lxc/config.yml`