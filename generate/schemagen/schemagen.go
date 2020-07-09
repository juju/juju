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
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"golang.org/x/tools/go/packages"

	"github.com/juju/juju/apiserver"
	commonerrors "github.com/juju/juju/apiserver/common/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/generate/schemagen/gen"
	"github.com/juju/juju/state"
)

func main() {
	var adminFacades = flag.Bool("admin-facades", false, "add the admin facades when generating the schema")

	flag.Parse()
	args := flag.Args()

	// the first argument here will be the name of the binary, so we ignore
	// argument 0 when looking for the filepath.
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "Expected one argument: filepath of json schema to save.")
		os.Exit(1)
	}

	result, err := gen.Generate(defaultPackages{
		path: "github.com/juju/juju/apiserver",
	}, defaultLinker{}, apiServerShim{}, gen.WithAdminFacades(*adminFacades))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	jsonSchema, err := json.MarshalIndent(result, "", "    ")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = ioutil.WriteFile(args[0], jsonSchema, 0644)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type apiServerShim struct{}

func (apiServerShim) AllFacades() gen.Registry {
	return apiserver.AllFacades()
}

func (apiServerShim) AdminFacadeDetails() []facade.Details {
	return apiserver.AdminFacadeDetails()
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

type defaultLinker struct{}

func (l defaultLinker) Links(facadeName string, factory facade.Factory) []string {
	var a []string
	for i, kindStr := range kinds {
		if l.isAvailable(facadeName, factory, entityKind(i)) {
			a = append(a, kindStr)
		}
	}
	return a
}

func (defaultLinker) isAvailable(facadeName string, factory facade.Factory, kind entityKind) (ok bool) {
	if factory == nil {
		// Admin facade only.
		return true
	}
	if kind == kindControllerUser && !apiserver.IsControllerFacade(facadeName) {
		return false
	}
	if kind == kindModelUser && !apiserver.IsModelFacade(facadeName) {
		return false
	}
	defer func() {
		err := recover()
		if err == nil {
			return
		}
		ok = true
	}()
	ctx := context{
		auth: authorizer{
			kind: kind,
		},
	}
	_, err := factory(ctx)
	return errors.Cause(err) != commonerrors.ErrPerm
}

type entityKind int

const (
	kindControllerMachine = entityKind(iota)
	kindMachineAgent
	kindUnitAgent
	kindControllerUser
	kindModelUser
)

func (k entityKind) String() string {
	return kinds[k]
}

var kinds = []string{
	kindControllerMachine: "controller-machine-agent",
	kindMachineAgent:      "machine-agent",
	kindUnitAgent:         "unit-agent",
	kindControllerUser:    "controller-user",
	kindModelUser:         "model-user",
}

type context struct {
	auth authorizer
	facade.Context
}

func (c context) Auth() facade.Authorizer {
	return c.auth
}

func (c context) ID() string {
	return ""
}

func (c context) State() *state.State {
	return new(state.State)
}

func (c context) Resources() facade.Resources {
	return nil
}

func (c context) StatePool() *state.StatePool {
	return new(state.StatePool)
}

func (c context) ControllerTag() names.ControllerTag {
	return names.NewControllerTag("xxxx")
}

type authorizer struct {
	facade.Authorizer
	kind entityKind
}

func (a authorizer) AuthController() bool {
	return a.kind == kindControllerMachine
}

func (a authorizer) HasPermission(operation permission.Access, target names.Tag) (bool, error) {
	return true, nil
}

func (a authorizer) AuthMachineAgent() bool {
	return a.kind == kindMachineAgent || a.kind == kindControllerMachine
}

func (a authorizer) AuthUnitAgent() bool {
	return a.kind == kindUnitAgent
}

func (a authorizer) AuthClient() bool {
	return a.kind == kindControllerUser || a.kind == kindModelUser
}

func (a authorizer) GetAuthTag() names.Tag {
	switch a.kind {
	case kindControllerUser, kindModelUser:
		return names.NewUserTag("bob")
	case kindUnitAgent:
		return names.NewUnitTag("xx/0")
	case kindMachineAgent, kindControllerMachine:
		return names.NewMachineTag("0")
	}
	panic("unknown kind")
}
