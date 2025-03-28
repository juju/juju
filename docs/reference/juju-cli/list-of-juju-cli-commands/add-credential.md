(command-juju-add-credential)=
# `juju add-credential`

```
Usage: juju add-credential [options] <cloud name>

Summary:
Adds a credential for a cloud to a local client and uploads it to a controller.

Global Options:
--debug  (= false)
    equivalent to --show-log --logging-config=<root>=DEBUG
-h, --help  (= false)
    Show help on a command or other topic.
--logging-config (= "")
    specify log levels for modules
--quiet  (= false)
    show no informational output
--show-log  (= false)
    if set, write the log file to stderr
--verbose  (= false)
    show more verbose output

Command Options:
-B, --no-browser-login  (= false)
    Do not use web browser for authentication
-c, --controller (= "")
    Controller to operate in
--client  (= false)
    Client operation
-f, --file (= "")
    The YAML file containing credentials to add
--local  (= false)
    DEPRECATED (use --client): Local operation only; controller not affected
--region (= "")
    Cloud region that credential is valid for
--replace  (= false)
    DEPRECATED: Overwrite existing credential information

Details:
The juju add-credential command operates in two modes.

When called with only the <cloud name> argument, `juju add-credential` will
take you through an interactive prompt to add a credential specific to
the cloud provider.

Providing the `-f <credentials.yaml>` option switches to the
non-interactive mode. <credentials.yaml> must be a path to a correctly
formatted YAML-formatted file.

Sample yaml file shows five credentials being stored against four clouds:

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

The <credential-name> parameter of each credential is arbitrary, but must
be unique within each <cloud-name>. This allows allow each cloud to store
multiple credentials.

The format for a credential is cloud-specific. Thus, it's best to use
'add-credential' command in an interactive mode. This will result in
adding this new credential locally and / or uploading it to a controller
in a correct format for the desired cloud.

The `--replace` option is required if credential information
for the named cloud already exists locally. All such information will be
overwritten. This option is DEPRECATED, use 'juju update-credential' instead.

Examples:
    juju add-credential google
    juju add-credential google --client
    juju add-credential google -c mycontroller
    juju add-credential aws -f ~/credentials.yaml -c mycontroller
    juju add-credential aws -f ~/credentials.yaml
    juju add-credential aws -f ~/credentials.yaml --client

Notes:
If you are setting up Juju for the first time, consider running
`juju autoload-credentials`. This may allow you to skip adding
credentials manually.

This command does not set default regions nor default credentials for the
cloud. The commands `juju default-region` and `juju default-credential`
provide that functionality.

Use --controller option to upload a credential to a controller.

Use --client option to add a credential to the current client.

Further help:
Please visit https://discourse.charmhub.io/t/1508 for cloud-specific
instructions.

See also:
    credentials
    remove-credential
    update-credential
    default-credential
    default-region
    autoload-credentials

```