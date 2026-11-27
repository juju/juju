(command-juju-storage-pools)=
# `juju storage-pools`
> See also: [create-storage-pool](#command-juju-create-storage-pool), [remove-storage-pool](#command-juju-remove-storage-pool)

**Aliases:** list-storage-pools

## Summary
List storage pools.

## Usage
```text
juju storage-pools [options]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--name` |  | Filter by pool name; repeat for each name |
| `-o`, `--output` |  | Specify an output file |
| `--provider` |  | Filter by provider; repeat for each provider |

## Examples

List the pools in the selected model:

    juju storage-pools

Show two pools by name:

    juju storage-pools --name rootfs --name tmpfs

On an AWS model, show the pools for the EBS and loop providers as YAML:

    juju storage-pools --provider ebs --provider loop --format yaml


## Details

Use `--name` to select a pool; a positional name does not filter the
list. To give several names or providers, repeat the corresponding flag.

The default table shows each pool's name, provider and attributes. In
JSON and YAML, the pool names are the keys. If no pools match, the
command succeeds without writing a table or a JSON or YAML document.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage-pool/