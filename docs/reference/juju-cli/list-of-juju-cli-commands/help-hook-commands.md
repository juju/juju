(command-juju-help-hook-commands)=
# `juju help-hook-commands`
> See also: [help](#help), [help-action-commands](#help-action-commands)

## Summary
Show help on a Juju charm hook command.

## Usage
```juju help-hook-commands [options] [hook]```

## Examples

For help on a specific hook command, supply the name of that hook command, for example:

    juju help-hook-commands unit-get


## Details

Juju charms have access to a set of built-in helpers known as 'hook-commands,'
which allow them to inspect their runtime environment.
The currently available charm hook commands include:

    application-version-set  Specifies which version of the application is deployed.
    close-port               Registers a request to close a port or port range.
    config-get               Prints application configuration.
    credential-get           Accesses cloud credentials.
    goal-state               Prints the status of the charm's peers and related units.
    is-leader                Prints application leadership status.
    juju-log                 Writes a message to Juju logs.
    juju-reboot              Reboots the host machine.
    k8s-raw-get              Gets Kubernetes raw spec information.
    k8s-raw-set              Sets k8s raw spec information.
    k8s-spec-get             Gets Kubernetes spec information.
    k8s-spec-set             Sets Kubernetes spec information.
    leader-get               Prints application leadership settings.
    leader-set               Writes application leadership settings.
    network-get              Gets network config.
    open-port                Registers a request to open a port or port range.
    opened-ports             Lists all ports or port ranges opened by the unit.
    payload-register         Registers a charm payload with Juju.
    payload-status-set       Updates the status of a payload.
    payload-unregister       Stops tracking a payload.
    pod-spec-get             Gets Kubernetes spec information. (deprecated)
    pod-spec-set             Sets Kubernetes spec information. (deprecated)
    relation-get             Get relation settings.
    relation-ids             Lists all relation IDs for the given endpoint.
    relation-list            Lists relation units.
    relation-model-get       Gets details about the model housing a related application.
    relation-set             Sets relation settings.
    resource-get             Gets the path to the locally cached resource file.
    secret-add               Adds a new secret.
    secret-get               Gets the content of a secret.
    secret-grant             Grants access to a secret.
    secret-ids               Prints secret IDs.
    secret-info-get          Gets a secret's metadata info.
    secret-remove            Removes an existing secret.
    secret-revoke            Revokes access to a secret.
    secret-set               Updates an existing secret.
    state-delete             Deletes server-side-state key-value pairs.
    state-get                Prints server-side-state value.
    state-set                Sets server-side-state values.
    status-get               Prints status information.
    status-set               Sets status information.
    storage-add              Adds storage instances.
    storage-get              Prints information for the storage instance with the specified ID.
    storage-list             Lists storage attached to the unit.
    unit-get                 Prints public-address or private-address.