(hook-command-payload-unregister)=
# `payload-unregister`

## Summary
Stop tracking a payload.

## Usage
``` payload-unregister [options] <class> <id>```

## Examples

    payload-unregister monitoring 0fcgaba


## Details

"payload-unregister" is used while a hook is running to let Juju know
that a payload has been manually stopped. The &lt;class&gt; and &lt;id&gt; provided
must match a payload that has been previously registered with juju using
payload-register.