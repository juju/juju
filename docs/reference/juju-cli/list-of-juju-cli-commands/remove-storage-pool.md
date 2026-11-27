(command-juju-remove-storage-pool)=
# `juju remove-storage-pool`
> See also: [create-storage-pool](#command-juju-create-storage-pool), [update-storage-pool](#command-juju-update-storage-pool), [storage-pools](#command-juju-storage-pools)

## Summary
Remove an existing storage pool.

## Usage
```text
juju remove-storage-pool [options] <name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju remove-storage-pool fast-storage


## Details

Give one pool name. The command does not prompt for confirmation and prints
nothing on success. If the pool is not found, the command reports the failure
and exits with an error.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage-pool/