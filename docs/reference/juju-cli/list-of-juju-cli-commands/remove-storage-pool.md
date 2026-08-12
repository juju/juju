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

Remove the storage-pool named fast-storage:

      juju remove-storage-pool fast-storage


## Details

Remove a single existing storage pool.