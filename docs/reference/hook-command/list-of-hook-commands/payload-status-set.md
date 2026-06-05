(hook-command-payload-status-set)=
# `payload-status-set`
## Summary
Updates the status of a payload.

## Usage
``` payload-status-set [options] <class> <id> <status>```

## Examples

    payload-status-set monitor abcd13asa32c starting


## Details

`payload-status-set` is used to update the current status of a registered payload.

The `<class>` and `<id>` provided must match a payload that has been previously
registered with Juju using `payload-register`.

The `<status>` must be one of the
follow: `starting`, `started`, `stopping`, `stopped`.