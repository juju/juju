(command-juju-help-tool)=
# `juju help-tool`
> See also: [help](#help)

## Summary
Show help on a Juju charm hook tool.

## Usage
```juju help-tool [options] [tool]```

## Examples

For help on a specific tool, supply the name of that tool, for example:

        juju help-tool unit-get


## Details

Juju charms can access a series of built-in helpers called 'hook-tools'.
These are useful for the charm to be able to inspect its running environment.
Currently available charm hook tools are:

    action-fail              Set action fail status with message.
    action-get               Get action parameters.
    action-log               Record a progress message for the current action.
    action-set               Set action results.
    application-version-set  Specify which version of the application is deployed.
    close-port               Register a request to close a port or port range.
    config-get               Print application configuration.
    credential-get           Access cloud credentials.
    goal-state               Print the status of the charm's peers and related units.
    is-leader                Print application leadership status.
    juju-log                 Write a message to the juju log.
    juju-reboot              Reboot the host machine.
    network-get              Get network config.
    open-port                Register a request to open a port or port range.
    opened-ports             List all ports or port ranges opened by the unit.
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