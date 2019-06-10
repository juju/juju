// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"reflect"

	"github.com/bcsaller/jsonschema"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/facade"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/rpc/rpcreflect"
)

// newDescribeAPICommand returns a full description of the api-servers
// AllFacades information as a JSON schema.
func newDescribeAPICommon() cmd.Command {
	return modelcmd.Wrap(&describeAPICommand{})
}

//go:generate mockgen -package commands -destination describeapi_mock.go github.com/juju/juju/cmd/juju/commands APIServer,Registry
type APIServer interface {
	AllFacades() Registry
	Close() error
}

type Registry interface {
	List() []facade.Description
	GetType(name string, version int) (reflect.Type, error)
}

type describeAPICommand struct {
	modelcmd.ModelCommandBase
	out       cmd.Output
	apiServer APIServer
}

const describeAPIHelpDoc = `
describe-api returns a full description of the api-servers AllFacades
information as a JSON schema. 

Examples:

	juju describe-api
`

// Info implements Command.
func (c *describeAPICommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "describe-api",
		Purpose: "Displays the JSON schema of the api-servers AllFacades.",
		Doc:     describeAPIHelpDoc,
	})
}

// SetFlags implements Command.
func (c *describeAPICommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "json", output.DefaultFormatters)
}

// Init implements Command.
func (c *describeAPICommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// Run implements Command.
func (c *describeAPICommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	registry := client.AllFacades()
	facades := registry.List()
	result := make([]FacadeSchema, len(facades))
	for i, facade := range facades {
		// select the latest version from the facade list
		version := facade.Versions[len(facade.Versions)-1]

		result[i].Name = facade.Name
		result[i].Version = version

		kind, err := registry.GetType(facade.Name, version)
		if err != nil {
			return errors.Annotatef(err, "getting type for facade %s at version %d", facade.Name, version)
		}
		objType := rpcreflect.ObjTypeOf(kind)
		result[i].Schema = jsonschema.ReflectFromObjType(objType)
	}
	return c.out.Write(ctx, result)
}

func (c *describeAPICommand) getAPI() (APIServer, error) {
	if c.apiServer != nil {
		return c.apiServer, nil
	}
	return apiServerShim{}, nil
}

type FacadeSchema struct {
	Name    string
	Version int
	Schema  *jsonschema.Schema
}

type apiServerShim struct{}

func (apiServerShim) AllFacades() Registry {
	return apiserver.AllFacades()
}

func (apiServerShim) Close() error {
	return nil
}
