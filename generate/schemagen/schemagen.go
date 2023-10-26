// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"golang.org/x/tools/go/packages"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/watcher/registry"
	schemagenformat "github.com/juju/juju/generate/schemagen/format"
	"github.com/juju/juju/generate/schemagen/gen"
	"github.com/juju/juju/state"
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
		output       = flag.String("output", "grpc", "output format (json, grpc)")
	)

	flag.Var(&facadeGroups, "facade-group", "facade group to export (latest, all, client, jimm)")
	flag.Parse()
	args := flag.Args()

	if len(facadeGroups) == 0 {
		facadeGroups = Strings([]string{"latest"})
	}

	format := strings.ToLower(*output)
	switch format {
	case "json", "grpc":
	case "":
		format = "json"
	default:
		fmt.Fprintf(os.Stderr, "Unknown output format %q\n", format)
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

	watcherRegistry, err := registry.NewRegistry(clock.WallClock)
	if err != nil {
		log.Fatalln(err)
	}
	linker := defaultLinker{
		watcherRegistry: watcherRegistry,
	}

	result, err := gen.Generate(defaultPackages{
		path: "github.com/juju/juju/apiserver",
	}, linker, apiServerShim{},
		gen.WithAdminFacades(*adminFacades),
		gen.WithFacadeGroups(groups),
	)
	if err != nil {
		log.Fatalln(err)
	}

	contents, err := schemagenformat.Format(format, result)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println(string(contents))
	return

	err = os.WriteFile(args[0], contents, 0644)
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

type defaultLinker struct {
	watcherRegistry facade.WatcherRegistry
}

func (l defaultLinker) Links(facadeName string, factory facade.Factory) []string {
	var a []string
	for i, kindStr := range kinds {
		if l.isAvailable(facadeName, factory, entityKind(i)) {
			a = append(a, kindStr)
		}
	}
	return a
}

func (l defaultLinker) isAvailable(facadeName string, factory facade.Factory, kind entityKind) (ok bool) {
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
		watcherRegistry: l.watcherRegistry,
	}
	_, err := factory(ctx)
	return errors.Cause(err) != apiservererrors.ErrPerm
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
	facade.Context
	auth            authorizer
	watcherRegistry facade.WatcherRegistry
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

func (c context) StatePool() *state.StatePool {
	return new(state.StatePool)
}

func (c context) Resources() facade.Resources {
	return common.NewResources()
}

func (c context) WatcherRegistry() facade.WatcherRegistry {
	return c.watcherRegistry
}

func (c context) ControllerTag() names.ControllerTag {
	return names.NewControllerTag("xxxx")
}

func (c context) Dispose() {}

func (c context) Cancel() <-chan struct{} {
	return make(chan struct{})
}

func (c context) ControllerDB() (changestream.WatchableDB, error) {
	return nil, nil
}

func (c context) DBDeleter() coredatabase.DBDeleter {
	return nil
}

type authorizer struct {
	facade.Authorizer
	kind entityKind
}

func (a authorizer) AuthController() bool {
	return a.kind == kindControllerMachine
}

func (a authorizer) HasPermission(operation permission.Access, target names.Tag) error {
	return nil
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
