# md-gen/model-config

This script generates a Markdown reference doc for the model config keys,
based on the code in the `github.com/juju/juju/environs/config` package.

It requires the following environment variables to be set:
- `DOCS_DIR`: the directory in which to place the outputted Markdown doc
- `JUJU_SRC_ROOT`: the root directory of a local checked-out copy of the
  Juju source tree (`github.com/juju/juju`). This is used to locate the
  `config` package and parse it into an AST to gather information e.g.
  doc comments.

Assuming you're in the root of the Juju source tree, you can invoke the script
like so:
```bash
export DOCS_DIR=$(pwd)/docs
export JUJU_SRC_ROOT=$(pwd)
go run ./scripts/md-gen/model-config
```