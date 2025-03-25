(hook-command-payload-status-set)=
# `payload-status-set`
## Summary
Update the status of a payload.

## Usage
``` payload-status-set [options] <class> <id> <status>```

## Examples

    payload-status-set monitor abcd13asa32c starting


## Details

"payload-status-set" is used to update the current status of a registered payload.
The &lt;class&gt; and &lt;id&gt; provided must match a payload that has been previously
registered with juju using payload-register. The &lt;status&gt; must be one of the
follow: starting, started, stopping, stopped