# md-gen/controller-config

This script generates a Markdown reference doc for the controller config keys,
based on the code in the `github.com/juju/juju/controller` package.

It requires one argument: the root directory of a local checked-out copy of the
Juju source tree (`github.com/juju/juju`). This is used to locate the
`controller` package and parse it into an AST to gather information e.g. doc
comments.

Assuming you're in the root of the Juju source tree, you can invoke the script
like so:
```bash
go run ./scripts/md-gen/controller-config $(pwd)
```