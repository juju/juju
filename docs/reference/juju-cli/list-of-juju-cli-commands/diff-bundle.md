(command-juju-diff-bundle)=
# `juju diff-bundle`

```
Usage: juju diff-bundle [options] <bundle file or name>

Summary:
Compare a bundle with a model and report any differences.

Global Options:
--debug  (= false)
    equivalent to --show-log --logging-config=<root>=DEBUG
-h, --help  (= false)
    Show help on a command or other topic.
--logging-config (= "")
    specify log levels for modules
--quiet  (= false)
    show no informational output
--show-log  (= false)
    if set, write the log file to stderr
--verbose  (= false)
    show more verbose output

Command Options:
-B, --no-browser-login  (= false)
    Do not use web browser for authentication
--annotations  (= false)
    Include differences in annotations
--arch (= "")
    specify an arch <all|amd64|arm64|armhf|i386|ppc64el|s390x>
--channel (= "")
    Channel to use when getting the bundle from the charm hub or charm store
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--map-machines (= "")
    Indicates how existing machines correspond to bundle machines
--overlay  (= )
    Bundles to overlay on the primary bundle, applied in order
--series (= "")
    specify a series

Details:
Bundle can be a local bundle file or the name of a bundle in
the charm store. The bundle can also be combined with overlays (in the
same way as the deploy command) before comparing with the model.

The map-machines option works similarly as for the deploy command, but
existing is always assumed, so it doesn't need to be specified.

Config values for comparison are always source from the "current" model
generation.

Examples:
    juju diff-bundle localbundle.yaml
    juju diff-bundle cs:canonical-kubernetes
    juju diff-bundle -m othermodel hadoop-spark
    juju diff-bundle cs:mongodb-cluster --channel beta
    juju diff-bundle cs:canonical-kubernetes --overlay local-config.yaml --overlay extra.yaml
    juju diff-bundle localbundle.yaml --map-machines 3=4

See also:
    deploy
```