(command-juju-diff-bundle)=
# `juju diff-bundle`
> See also: [deploy](#deploy)

## Summary
Compares a bundle with a model and reports any differences.

## Usage
```juju diff-bundle [options] <bundle file or name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--annotations` | false | Specifies whether to include differences in annotations. |
| `--arch` |  | Specifies an arch &lt;all&#x7c;amd64&#x7c;arm64&#x7c;ppc64el&#x7c;riscv64&#x7c;s390x&gt;. |
| `--base` |  | Specifies a base. |
| `--channel` |  | Specifies the channel to use when getting the bundle from Charmhub. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `--map-machines` |  | Indicates how existing machines correspond to bundle machines. |
| `--overlay` |  | Specifies bundles to overlay on the primary bundle, applied in order. |
| `--series` |  | (DEPRECATED) Specifies a series. Deprecated: Use `--base` instead. |

## Examples

    juju diff-bundle localbundle.yaml
    juju diff-bundle charmed-kubernetes
    juju diff-bundle charmed-kubernetes --overlay local-config.yaml --overlay extra.yaml
	juju diff-bundle charmed-kubernetes --base ubuntu@22.04
    juju diff-bundle -m othermodel hadoop-spark
    juju diff-bundle localbundle.yaml --map-machines 3=4


## Details

A bundle can be a local bundle file or the name of a bundle in
Charmhub. The bundle can also be combined with overlays (in the
same way as the deploy command) before comparing with the model.

The `--map-machines` option works similarly as for the `deploy` command, but
existing is always assumed, so it doesn't need to be specified.

Config values for comparison are always sourced from the current model
generation.

Specifying a base will retrieve the bundle for the relevant store for
the given base.