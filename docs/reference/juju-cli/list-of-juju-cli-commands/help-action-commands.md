(command-juju-help-action-commands)=
# `juju help-action-commands`
> See also: [help](#help), [help-hook-commands](#help-hook-commands)

## Summary
Show help on a Juju charm action command.

## Usage
```juju help-action-commands [options] [action]```

## Examples

For help on a specific action command, supply the name of that action command, for example:

        juju help-action-commands action-fail


## Details

In addition to hook commands, Juju charms also have access to a set of action-specific commands. 
These action commands are available when an action is running, and are used to log progress
and report the outcome of the action.
The currently available charm action commands include:
    action-fail  Set action fail status with message.
    action-get   Get action parameters.
    action-log   Record a progress message for the current action.
    action-set   Set action results.