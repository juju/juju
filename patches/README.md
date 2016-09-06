Emergency patches for dependencies
==================================

Files in this directory named `*.patch` or `*.diff` will be applied to
the source tree before building Juju for release. The expectation is
that these should be changes that will be accepted upstream, but that
we need to apply sooner.

They're applied with `$GOPATH/src` as the current directory, and with
`-p1` to strip one component off the start of the file path.

For more details, see `lp:juju-release-tools/apply_patches.py` or ask
babbageclunk or mgz in IRC.
