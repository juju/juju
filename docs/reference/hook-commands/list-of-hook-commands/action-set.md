(hook-command-action-set)=
# `action-set`

## Summary
Set action results.

## Usage
``` action-set [options] <key-path>=<value> [<key-path>=<value> ...]```

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

action-set adds the given values to the results map of the action. This map
is returned to the user after the completion of the action.
Keys must be given as a flat period-separated path of keys.
Each key must start and end with lowercase alphanumeric,
and contain only lowercase alphanumeric and hyphens.
Examples of valid key paths:
["foo", "500", "5-o-0", "foo.bar", "foo.bar.baz", "foo-bar.baz"]
Examples of invalid key paths:
["-foo", "foo-", "foo-.bar", "foo!bar", "foo..bar", ".foo", "foo.", ".", ""]
The following special keys are reserved for internal use, and thus not allowed:
"stdout", "stdout-encoding", "stderr", "stderr-encoding".
Values are always interpreted as strings.
The final result will be a nested object containing the merged results,
with any conflicting values overwriting previous values.