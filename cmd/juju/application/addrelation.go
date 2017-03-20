// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"regexp"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/featureflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
)

const addRelationDocCrossModel = `
Add a relation between 2 local application endpoints or a local endpoint and a remote application endpoint.
Adding a relation between two remote application endpoints is not supported.

Application endpoints can be identified either by:
    <application name>[:<relation name>]
        where application name supplied without relation will be internally expanded to be well-formed
or
    <model name>.<application name>[:<relation name>]
        where the application is hosted in another model in the same controller
or
    <user name>/<model name>.<application name>[:<relation name>]
        where model name is another model in the same controller and in this case has been disambiguated
        by prefixing with the model owner

Examples:
    $ juju add-relation wordpress mysql
        where "wordpress" and "mysql" will be internally expanded to "wordpress:mysql" and "mysql:server" respectively

    $ juju add-relation wordpress prod.db2
        where "wordpress" will be internally expanded to "wordpress:db2"

`

var localEndpointRegEx = regexp.MustCompile("^" + names.RelationSnippet + "$")

// NewAddRelationCommand returns a command to add a relation between 2 services.
func NewAddRelationCommand() cmd.Command {
	cmd := &addRelationCommand{}
	cmd.newAPIFunc = func() (ApplicationAddRelationAPI, error) {
		root, err := cmd.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return application.NewClient(root), nil

	}
	return modelcmd.Wrap(cmd)
}

// addRelationCommand adds a relation between two application endpoints.
type addRelationCommand struct {
	modelcmd.ModelCommandBase
	Endpoints      []string
	remoteEndpoint string
	newAPIFunc     func() (ApplicationAddRelationAPI, error)
}

func (c *addRelationCommand) Info() *cmd.Info {
	addCmd := &cmd.Info{
		Name:    "add-relation",
		Aliases: []string{"relate"},
		Args:    "<application1>[:<endpoint name1>] <application2>[:<endpoint name2>]",
		Purpose: "Add a relation between two application endpoints.",
	}
	if featureflag.Enabled(feature.CrossModelRelations) {
		addCmd.Doc = addRelationDocCrossModel
	}
	return addCmd
}

func (c *addRelationCommand) Init(args []string) error {
	if len(args) != 2 {
		return errors.Errorf("a relation must involve two applications")
	}
	if err := c.validateEndpoints(args); err != nil {
		return err
	}
	c.Endpoints = args
	return nil
}

// ApplicationAddRelationAPI defines the API methods that application add relation command uses.
type ApplicationAddRelationAPI interface {
	Close() error
	// CHECK
	BestAPIVersion() int
	AddRelation(endpoints ...string) (*params.AddRelationResults, error)
}

func (c *addRelationCommand) Run(ctx *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	if c.remoteEndpoint != "" && client.BestAPIVersion() < 3 {
		// old client does not have cross-model capability.
		return errors.NotSupportedf("cannot add relation between %s: remote endpoints", c.Endpoints)
	}

	_, err = client.AddRelation(c.Endpoints...)
	if params.IsCodeUnauthorized(err) {
		common.PermissionsMessage(ctx.Stderr, "add a relation")
	}
	if err == nil && c.remoteEndpoint != "" {
		ctx.Infof("Note: this beta version of Juju has automatically exposed the endpoint at %s to enable cross model communications", c.remoteEndpoint)
	}
	return block.ProcessBlockedError(err, block.BlockChange)
}

// validateEndpoints determines if all endpoints are valid.
// Each endpoint is either from local service or remote.
// If more than one remote endpoint are supplied, the input argument are considered invalid.
func (c *addRelationCommand) validateEndpoints(all []string) error {
	for _, endpoint := range all {
		if featureflag.Enabled(feature.CrossModelRelations) {
			// We can only determine if this is a remote endpoint with 100%.
			// If we cannot parse it, it may still be a valid local endpoint...
			// so ignoring parsing error,
			if _, err := crossmodel.ParseApplicationURL(endpoint); err == nil {
				if c.remoteEndpoint != "" {
					return errors.NotSupportedf("providing more than one remote endpoints")
				}
				c.remoteEndpoint = endpoint
				continue
			}
		}
		// at this stage, we are assuming that this could be a local endpoint
		if err := validateLocalEndpoint(endpoint, ":"); err != nil {
			return err
		}
	}
	return nil
}

// validateLocalEndpoint determines if given endpoint could be a valid
func validateLocalEndpoint(endpoint string, sep string) error {
	i := strings.Index(endpoint, sep)
	applicationName := endpoint
	if i != -1 {
		// not a valid endpoint as sep either at the start or the end of the name
		if i == 0 || i == len(endpoint)-1 {
			return errors.NotValidf("endpoint %q", endpoint)
		}

		parts := strings.SplitN(endpoint, sep, -1)
		if rightCount := len(parts) == 2; !rightCount {
			// not valid if there are not exactly 2 parts.
			return errors.NotValidf("endpoint %q", endpoint)
		}

		applicationName = parts[0]

		if valid := localEndpointRegEx.MatchString(parts[1]); !valid {
			return errors.NotValidf("endpoint %q", endpoint)
		}
	}

	if valid := names.IsValidApplication(applicationName); !valid {
		return errors.NotValidf("application name %q", applicationName)
	}
	return nil
}
