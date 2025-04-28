// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"strings"

	"github.com/juju/errors"
	"golang.org/x/tools/go/packages"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/generate/schemagen/gen"
)

// Strings represents a way to have multiple values passed to the flags
// cmd -config=a -config=b
type Strings []string

// Set will append a config value to the config flags.
func (c *Strings) Set(value string) error {
	parts := strings.Split(value, ",")
	for _, part := range parts {
		*c = append(*c, part)
	}
	return nil
}

func (c *Strings) String() string {
	return strings.Join(*c, ",")
}

func main() {
	var (
		facadeGroups Strings
		adminFacades = flag.Bool("admin-facades", false, "add the admin facades when generating the schema")
		comments     = flag.Bool("comments", false, "add comments from go source")
	)

	flag.Var(&facadeGroups, "facade-group", "facade group to export (latest, all, client, agent, jimm)")
	flag.Parse()
	args := flag.Args()

	if len(facadeGroups) == 0 {
		facadeGroups = Strings([]string{"latest"})
	}

	// the first argument here will be the name of the binary, so we ignore
	// argument 0 when looking for the filepath.
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "Expected one argument: filepath of json schema to save.")
		os.Exit(1)
	}

	unique := make(map[gen.FacadeGroup]struct{})
	for _, s := range facadeGroups {
		group, err := gen.ParseFacadeGroup(s)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		unique[group] = struct{}{}
	}
	groups := make([]gen.FacadeGroup, 0, len(unique))
	for g := range unique {
		groups = append(groups, g)
	}

	var packages gen.PackageRegistry
	if *comments {
		// Resolving comments requires being able to load the go source.
		packages = defaultPackages{
			path: "github.com/juju/juju/apiserver",
		}
	} else {
		packages = noPackages{}
	}

	result, err := gen.Generate(packages, apiServerShim{},
		gen.WithAdminFacades(*adminFacades),
		gen.WithFacadeGroups(groups),
	)
	if err != nil {
		log.Fatalln(err)
	}

	jsonSchema, err := json.MarshalIndent(result, "", "    ")
	if err != nil {
		log.Fatalln(err)
	}

	err = os.WriteFile(args[0], jsonSchema, 0644)
	if err != nil {
		log.Fatalln(err)
	}
}

type apiServerShim struct{}

func (apiServerShim) AllFacades() gen.Registry {
	return apiserver.AllFacades()
}

func (apiServerShim) AdminFacadeDetails() []facade.Details {
	return apiserver.AdminFacadeDetails()
}

type noPackages struct{}

func (p noPackages) LoadPackage() (*packages.Package, error) {
	return nil, nil
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
