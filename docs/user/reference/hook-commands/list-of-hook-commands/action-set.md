(hook-command-action-set)=
# `action-set`
## Summary
Set action results.

## Usage
``` action-set [options] <key>=<value> [<key>=<value> ...]```

## Examples

    action-set outfile.size=10G
    action-set foo.bar=2
    action-set foo.baz.val=3
    action-set foo.bar.zab=4
    action-set foo.baz=1

will yield:

    outfile:
      size: "10G"
    foo:
      bar:
        zab: "4"
      baz: "1"


## Details

action-set adds the given values to the results map of the Action. This map
is returned to the user after the completion of the Action. Keys must start
and end with lowercase alphanumeric, and contain only lowercase alphanumeric,
hyphens and periods.  The following special keys are reserved for internal use: 
"stdout", "stdout-encoding", "stderr", "stderr-encoding".