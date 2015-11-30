// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/model/crossmodel"
)

const addRelationDoc = `
Add a relation between 2 local service endpoints or a local and a remote service endpoints.
Adding a relation between two remote service endpoints is not supported.

Service endpoints can be identified either by:
    <service name>[:<relation name>]
or
    <remote endpoint url>

Examples:
    $ juju add-relation wordpress mysql
    $ juju add-relation wordpress local:/u/fred/db2

`

var localEndpointRegEx = regexp.MustCompile("^" + names.RelationSnippet + "$")

func newAddRelationCommand() cmd.Command {
	addRelationCmd := &addRelationCommand{}
	addRelationCmd.newAPIFunc = func() (AddRelationAPI, error) {
		client, err := addRelationCmd.NewAPIClient()
		if err != nil {
			return nil, err
		}
		return client, nil
	}
	return envcmd.Wrap(addRelationCmd)
}

// addRelationCommand adds a relation between two service endpoints.
type addRelationCommand struct {
	envcmd.EnvCommandBase
	Endpoints []string

	out                cmd.Output
	hasRemoteEndpoints bool
	newAPIFunc         func() (AddRelationAPI, error)
}

func (c *addRelationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-relation",
		Args:    "<endpoint 1> <endpoint 2>",
		Purpose: "add a relation between two service endpoints",
		Doc:     addRelationDoc,
	}
}

func (c *addRelationCommand) Init(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("a relation must involve two service endpoints")
	}

	if err := c.validateEndpoints(args); err != nil {
		return err
	}
	c.Endpoints = args
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *addRelationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters)
}

func (c *addRelationCommand) Run(ctx *cmd.Context) error {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	if api.BestAPIVersion() < 2 {
		if c.hasRemoteEndpoints {
			// old client does not have cross-model capability.
			return errors.NotSupportedf("cannot add relation between %s: remote endpoints", c.Endpoints)
		}
	}
	added, err := api.AddRelation(c.Endpoints...)
	if blockedErr := block.ProcessBlockedError(err, block.BlockChange); blockedErr != nil {
		return blockedErr
	}
	return c.out.Write(ctx, added)
}

// validateEndpoints determines if all endpoints are valid.
// Each endpoint is either from local service or remote.
// If more than one remote endpoint are supplied, the input argument are considered invalid.
func (c *addRelationCommand) validateEndpoints(all []string) error {
	for _, endpoint := range all {
		// We can only determine if this is a remote endpoint with 100%.
		// If we cannot parse it, it may still be a valid local endpoint...
		// so ignoring parsing error,
		if _, err := crossmodel.ParseServiceURL(endpoint); err == nil {
			if c.hasRemoteEndpoints {
				return errors.NotSupportedf("providing more than one remote endpoints")
			}
			c.hasRemoteEndpoints = true
			continue
		}
		// at this stage, we are asuming that this could be a local endpoint
		if err := validateLocalEndpoint(endpoint, ":"); err != nil {
			return err
		}
	}
	return nil
}

// validateLocalEndpoint determines if given endpoint could be a valid
func validateLocalEndpoint(endpoint string, sep string) error {
	i := strings.Index(endpoint, sep)
	serviceName := endpoint
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

		serviceName = parts[0]

		if valid := localEndpointRegEx.MatchString(parts[1]); !valid {
			return errors.NotValidf("endpoint %q", endpoint)
		}
	}

	if valid := names.IsValidService(serviceName); !valid {
		return errors.NotValidf("service name %q", serviceName)
	}
	return nil
}

// AddRelationAPI defines the API methods that can add relation between services.
type AddRelationAPI interface {
	Close() error
	BestAPIVersion() int
	AddRelation(endpoints ...string) (*params.AddRelationResults, error)
}
