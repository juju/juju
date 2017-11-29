Patches for dependencies
==================================

Files in this directory named `*.diff` will be applied to the source tree
before building Juju for release. The expectation is that these changes have
been submitted upstream, but are delayed in being accepted.

They're applied with `$GOPATH/src` as the current directory, and with
`-p1` to strip one component off the start of the file path.

The patches will be applied automatically when you build juju with the
Makefile, or snapcraft.
