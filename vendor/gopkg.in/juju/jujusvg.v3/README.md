jujusvg
=======

A library for generating SVGs from Juju bundles and environments.

Installation
------------

To start using jujusvg, first ensure you have a valid Go environment, then run
the following:

    go get gopkg.in/juju/jujusvg.v2

Dependencies
------------

The project uses godeps (https://launchpad.net/godeps) to manage Go
dependencies. To install this, run:


    go get github.com/rogpeppe/godeps

After installing it, you can update the dependencies to the revision specified
in the `dependencies.tsv` file with the following:

    make deps

Use `make create-deps` to update the dependencies file.

Usage
-----

Given a Juju bundle, you can convert this to an SVG programatically.  This
generates a simple SVG representation of a bundle or bundles that can then be
included in a webpage as a visualization.

For an example of how to use this library, please see `examples/generatesvg.go`.
You can run this example like:

    go run generatesvg.go bundle.yaml > bundle.svg

The examples directory also includes three sample bundles that you can play
around with, or you can use the [Juju GUI](https://demo.jujucharms.com) to
generate your own bundles.

Design-related assets
---------------------

Some assets are specified based on assets provided by the design team. These
assets are specified in the defs section of the generated SVG, and can thus
be found in the Canvas.definition() method.  These assets are, except where
indicated, embedded in a go file assigned to an exported variable, so that they
may be used like so:

```go
import (
	"io"

	"gopkg.in/juju/jujusvg.v2/assets"
)

// ...

io.WriteString(canvas.Writer, assets.AssetToWrite)
```

Current assets in use:

* ~~The service block~~ *the service block has been deprecated and is now handled with SVGo*
* The relation health indicator
