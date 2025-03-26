(hook-command-leader-set)=
# `leader-set`
## Summary
Write application leadership settings.

## Usage
``` leader-set [options] <key>=<value> [...]```

## Examples

    leader-set cluster-leader-address=10.0.0.123


## Details

leader-set immediate writes the supplied key/value pairs to the controller,
which will then inform non-leader units of the change. It will fail if called
without arguments, or if called by a unit that is not currently application leader.

leader-set lets you distribute string key=value pairs to other units, but with the
following differences:
    there’s only one leader-settings bucket per application (not one per unit)
    only the leader can write to the bucket
    only minions are informed of changes to the bucket
    changes are propagated instantly

The instant propagation may be surprising, but it exists to satisfy the use case where
shared data can be chosen by the leader at the very beginning of the install hook.

It is strongly recommended that leader settings are always written as a self-consistent
group leader-set one=one two=two three=three.