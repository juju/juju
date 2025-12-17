(command-juju-storage-pools)=
# `juju storage-pools`
> See also: [create-storage-pool](#create-storage-pool), [remove-storage-pool](#remove-storage-pool)

**Aliases:** list-storage-pools

## Summary
Lists storage pools.

## Usage
```juju storage-pools [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `--name` |  | Specifies the names to restrict pools to. |
| `-o`, `--output` |  | Specify an output file |
| `--provider` |  | Specifies the provider types to restrict pools to. |

## Examples

List all storage pools:

    juju storage-pools

List only pools of type kubernetes, azure, ebs:

    juju storage-pools --provider kubernetes,azure,ebs

List only pools named pool1 and pool2:

    juju storage-pools --name pool1,pool2


## Details

Lists storage pools. Filters on pool type, name.

If no filter is specified, all current pools are listed.
If at least 1 name and type is specified, only pools that match both a name
AND a type from criteria are listed.
If only names are specified, only mentioned pools will be listed.
If only types are specified, all pools of the specified types will be listed.

Both pool types and names must be valid.
Valid pool types are pool types that are registered for Juju model.