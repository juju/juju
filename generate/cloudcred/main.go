// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"runtime/debug"
	"text/template"

	"github.com/juju/juju/environs"
	_ "github.com/juju/juju/internal/provider/all"
	_ "github.com/juju/juju/internal/provider/dummy"
)

var file = flag.String("o", "", "`file` to write.")
var packageName = flag.String("p", "cloudcred", "package name for generated file")

func main() {
	flag.Parse()

	visibleAttributes := make(map[string]bool)
	for _, pname := range environs.RegisteredProviders() {
		p, err := environs.Provider(pname)
		if err != nil {
			panic(err)
		}
		for authtype, s := range p.CredentialSchemas() {
			for _, attr := range s {
				visibleAttributes[fmt.Sprintf("%s,%s,%s", pname, authtype, attr.Name)] = !attr.Hidden
			}
		}
	}

	p := params{
		PackageName: *packageName,
		Attributes:  visibleAttributes,
	}

	bi, ok := debug.ReadBuildInfo()
	if ok {
		for _, d := range bi.Deps {
			if d.Path != "github.com/juju/juju" {
				continue
			}
			if d.Replace != nil {
				break
			}
			break
		}
	}

	b := new(bytes.Buffer)
	if err := tmpl.Execute(b, p); err != nil {
		panic(err)
	}

	formatted, err := format.Source(b.Bytes())
	if err != nil {
		panic(err)
	}

	if *file != "" {
		if err := os.WriteFile(*file, formatted, 0664); err != nil {
			panic(err)
		}
	} else {
		os.Stdout.Write(formatted)
	}
}

type params struct {
	PackageName string
	Attributes  map[string]bool
}

var tmpl = template.Must(template.New("").Parse(`
// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
//
// GENERATED FILE - DO NOT EDIT

package {{.PackageName}}

var attr = map[string]bool {
{{range $name, $value := .Attributes}}	{{printf "%q" $name}}: {{$value}},
{{end -}}
}
`[1:]))
