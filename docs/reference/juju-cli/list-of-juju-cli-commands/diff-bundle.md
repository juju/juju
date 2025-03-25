(command-juju-diff-bundle)=
# `juju diff-bundle`
> See also: [deploy](#deploy)

## Summary
Compare a bundle with a model and report any differences.

## Usage
```juju diff-bundle [options] <bundle file or name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--annotations` | false | Include differences in annotations |
| `--arch` |  | specify an arch &lt;all&#x7c;amd64&#x7c;arm64&#x7c;ppc64el&#x7c;riscv64&#x7c;s390x&gt; |
| `--base` |  | specify a base |
| `--channel` |  | Channel to use when getting the bundle from Charmhub |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--map-machines` |  | Indicates how existing machines correspond to bundle machines |
| `--overlay` |  | Bundles to overlay on the primary bundle, applied in order |
| `--series` |  | specify a series. DEPRECATED: use --base |

## Examples

    juju diff-bundle localbundle.yaml
    juju diff-bundle charmed-kubernetes
    juju diff-bundle charmed-kubernetes --overlay local-config.yaml --overlay extra.yaml
	juju diff-bundle charmed-kubernetes --base ubuntu@22.04
    juju diff-bundle -m othermodel hadoop-spark
    juju diff-bundle localbundle.yaml --map-machines 3=4


## Details

Bundle can be a local bundle file or the name of a bundle in
Charmhub. The bundle can also be combined with overlays (in the
same way as the deploy command) before comparing with the model.

The map-machines option works similarly as for the deploy command, but
existing is always assumed, so it doesn't need to be specified.

Config values for comparison are always source from the "current" model
generation.

Specifying a base will retrieve the bundle for the relevant store for
the give base.