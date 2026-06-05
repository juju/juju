(command-juju-add-credential)=
# `juju add-credential`
> See also: [credentials](#credentials), [remove-credential](#remove-credential), [update-credential](#update-credential), [default-credential](#default-credential), [default-region](#default-region), [autoload-credentials](#autoload-credentials)

## Summary
Adds a credential for a cloud to a local client and uploads it to a controller.

## Usage
```juju add-credential [options] <cloud name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |
| `-f`, `--file` |  | The YAML file containing credentials to add |
| `--region` |  | Cloud region that credential is valid for |

## Examples

    juju add-credential google
    juju add-credential google --client
    juju add-credential google -c mycontroller
    juju add-credential aws -f ~/credentials.yaml -c mycontroller
    juju add-credential aws -f ~/credentials.yaml
    juju add-credential aws -f ~/credentials.yaml --client


## Details

The `juju add-credential`command operates in two modes.

When called with only the `<cloud name>` argument, `juju add-credential` will
take you through an interactive prompt to add a credential specific to
the cloud provider.

Providing the `-f <credentials.yaml>` option switches to the
non-interactive mode. `<credentials.yaml>` must be a path to a correctly
formatted YAML-formatted file.

The following sample YAML file shows five credentials being stored against four clouds:

    credentials:
      aws:
        <credential-name>:
          auth-type: access-key
          access-key: <key>
          secret-key: <key>
      azure:
        <credential-name>:
          auth-type: service-principal-secret
          application-id: <uuid>
          application-password: <password>
          subscription-id: <uuid>
      lxd:
        <credential-a>:
          auth-type: interactive
          trust-password: <password>
        <credential-b>:
          auth-type: interactive
          trust-password: <password>
      google:
        <credential-name>:
          auth-type: oauth2
          project-id: <project-id>
          private-key: <private-key>
          client-email: <email>
          client-id: <client-id>

The `<credential-name>` parameter of each credential is arbitrary, but must
be unique within each `<cloud-name>`. This allows each cloud to store
multiple credentials.

The format for a credential is cloud-specific. Thus, it's best to use
the `add-credential` command in an interactive mode. This will result in
adding this new credential locally and / or uploading it to a controller
in a correct format for the desired cloud.


Notes:
If you are setting up Juju for the first time, consider running
`juju autoload-credentials`. This may allow you to skip adding
credentials manually.

This command does not set default regions nor default credentials for the
cloud. The commands `juju default-region`  and `juju default-credential`
provide that functionality.