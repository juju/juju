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

    add-metric               Add metrics.
    application-version-set  Specify which version of the application is deployed.
    close-port               Register a request to close a port or port range.
    config-get               Print application configuration.
    credential-get           Access cloud credentials.
    goal-state               Print the status of the charm's peers and related units.
    is-leader                Print application leadership status.
    juju-log                 Write a message to the juju log.
    juju-reboot              Reboot the host machine.
    k8s-raw-get              Get k8s raw spec information.
    k8s-raw-set              Set k8s raw spec information.
    k8s-spec-get             Get k8s spec information.
    k8s-spec-set             Set k8s spec information.
    leader-get               Print application leadership settings.
    leader-set               Write application leadership settings.
    network-get              Get network config.
    open-port                Register a request to open a port or port range.
    opened-ports             List all ports or port ranges opened by the unit.
    payload-register         Register a charm payload with Juju.
    payload-status-set       Update the status of a payload.
    payload-unregister       Stop tracking a payload.
    pod-spec-get             Get k8s spec information. (deprecated)
    pod-spec-set             Set k8s spec information. (deprecated)
    relation-get             Get relation settings.
    relation-ids             List all relation IDs for the given endpoint.
    relation-list            List relation units.
    relation-model-get       Get details about the model hosing a related application.
    relation-set             Set relation settings.
    resource-get             Get the path to the locally cached resource file.
    secret-add               Add a new secret.
    secret-get               Get the content of a secret.
    secret-grant             Grant access to a secret.
    secret-ids               Print secret IDs.
    secret-info-get          Get a secret's metadata info.
    secret-remove            Remove an existing secret.
    secret-revoke            Revoke access to a secret.
    secret-set               Update an existing secret.
    state-delete             Delete server-side-state key value pairs.
    state-get                Print server-side-state value.
    state-set                Set server-side-state values.
    status-get               Print status information.
    status-set               Set status information.
    storage-add              Add storage instances.
    storage-get              Print information for the storage instance with the specified ID.
    storage-list             List storage attached to the unit.
    unit-get                 Print public-address or private-address.