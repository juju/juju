(hook-command-action-fail)=
# `action-fail`

## Summary
Set action fail status with message.

## Usage
``` action-fail [options] ["<failure message>"]```

## Examples

    action-fail 'unable to contact remote service'


## Details

action-fail sets the fail state of the action with a given error message.  Using
action-fail without a failure message will set a default message indicating a
problem with the action.