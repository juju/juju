// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/generate/schemagen/gen"
	"golang.org/x/tools/go/packages"
)

func main() {
	// the first argument here will be the name of the binary, so we ignore
	// argument 0 when looking for the filepath.
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Expected one argument: filepath of json schema to save.")
		os.Exit(1)
	}

	result, err := gen.Generate(defaultPackages{
		path: "github.com/juju/juju/apiserver",
	}, apiServerShim{})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	jsonSchema, err := json.MarshalIndent(result, "", "    ")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = ioutil.WriteFile(os.Args[1], jsonSchema, 0644)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type apiServerShim struct{}

func (apiServerShim) AllFacades() gen.Registry {
	return apiserver.AllFacades()
}

type defaultPackages struct {
	path string
}

func (p defaultPackages) LoadPackage() (*packages.Package, error) {
	cfg := packages.Config{
		Mode: packages.LoadAllSyntax,
		ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
			return parser.ParseFile(fset, filename, src, parser.ParseComments)
		},
	}

	pkgs, err := packages.Load(&cfg, p.path)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot load %q", p.path)
	}
	if len(pkgs) != 1 {
		return nil, errors.Errorf("packages.Load returned %d packages, not 1", len(pkgs))
	}
	return pkgs[0], nil
}
