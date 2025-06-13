// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/spaces"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
)

// NewBindCommand returns a command which changes the bindings for an application.
func NewBindCommand() cmd.Command {
	cmd := &bindCommand{
		NewApplicationClient: func(conn base.APICallCloser) ApplicationBindClient {
			return application.NewClient(conn)
		},
		NewSpacesClient: func(conn base.APICallCloser) SpacesAPI {
			return spaces.NewAPI(conn)
		},
	}
	return modelcmd.Wrap(cmd)
}

// ApplicationBindClient defines a subset of the application facade that deals with
// querying and updating application bindings.
type ApplicationBindClient interface {
	Get(string, string) (*params.ApplicationGetResults, error)
	MergeBindings(req params.ApplicationMergeBindingsArgs) error
}

// Bind is responsible for changing the bindings for an application.
type bindCommand struct {
	modelcmd.ModelCommandBase

	NewApplicationClient func(base.APICallCloser) ApplicationBindClient
	NewSpacesClient      func(base.APICallCloser) SpacesAPI

	ApplicationName string
	BindExpression  string
	Bindings        map[string]string
	Force           bool
}

const bindCmdDoc = `
In order to be able to bind any endpoint to a space, all machines where the
application units are deployed to are required to be configured with an address
in that space. However, you can use the --force option to bypass this check.

Examples:

To update the default binding for the application and automatically update all
existing endpoint bindings that were referencing the old default, you can use 
the following syntax:

  juju bind foo new-default

To bind individual endpoints to a space you can use the following syntax:

  juju bind foo endpoint-1=space-1 endpoint-2=space-2

Finally, the above commands can be combined to update both the default space
and individual endpoints in one go:

  juju bind foo new-default endpoint-1=space-1
`

func (c *bindCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "bind",
		Args:    "<application> [<default-space>] [<endpoint-name>=<space> ...]",
		Purpose: "Change bindings for a deployed application.",
		Doc:     bindCmdDoc,
	})
}

func (c *bindCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.Force, "force", false, "Allow endpoints to be bound to spaces that might not be available to all existing units")
}

func (c *bindCommand) Init(args []string) error {
	nArgs := len(args)
	if nArgs == 0 {
		return errors.Errorf("no application specified")
	}

	if !names.IsValidApplication(args[0]) {
		return errors.Errorf("invalid application name %q", args[0])
	}
	c.ApplicationName = args[0]
	c.BindExpression = strings.Join(args[1:], " ")
	return nil
}

// Run connects to the specified environment and applies the requested binding
// changes.
func (c *bindCommand) Run(ctx *cmd.Context) error {
	apiRoot, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = apiRoot.Close() }()

	if err = c.checkApplicationFacadeSupport(apiRoot, "changing application bindings", 11); err != nil {
		return err
	}

	if err = c.parseBindExpression(apiRoot); err != nil && errors.IsNotSupported(err) {
		ctx.Infof("Spaces not supported by this model's cloud, nothing to do.")
		return nil
	} else if err != nil {
		return err
	}

	if len(c.Bindings) == 0 {
		return errors.New("no bindings specified")
	}

	generation, err := c.ActiveBranch()
	if err != nil {
		return errors.Trace(err)
	}

	applicationClient := c.NewApplicationClient(apiRoot)
	applicationInfo, err := applicationClient.Get(generation, c.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}

	// Validate endpoints and merge operator-defined bindings
	curBindings := applicationInfo.EndpointBindings
	var epList []string
	for epName := range curBindings {
		if epName == "" {
			continue
		}
		epList = append(epList, epName)
	}
	curCharmEndpoints := set.NewStrings(epList...)

	if err := c.validateEndpointNames(curCharmEndpoints, curBindings, c.Bindings); err != nil {
		return errors.Trace(err)
	}

	appDefaultSpace := curBindings[""]

	var bindingsChangelog []string
	c.Bindings, bindingsChangelog = mergeBindings(curCharmEndpoints, curBindings, c.Bindings, appDefaultSpace)

	err = applicationClient.MergeBindings(params.ApplicationMergeBindingsArgs{
		Args: []params.ApplicationMergeBindings{
			{
				ApplicationTag: names.NewApplicationTag(c.ApplicationName).String(),
				Bindings:       c.Bindings,
				Force:          c.Force,
			},
		},
	})
	if err != nil {
		return errors.Trace(err)
	}

	// Emit binding changelog after a successful call to MergeBindings.
	for _, change := range bindingsChangelog {
		ctx.Infof("%s", change)
	}
	return nil
}

func (c *bindCommand) validateEndpointNames(newCharmEndpoints set.Strings, oldEndpointsMap, userBindings map[string]string) error {
	for epName := range userBindings {
		if _, exists := oldEndpointsMap[epName]; exists || epName == "" {
			continue
		}

		if !newCharmEndpoints.Contains(epName) {
			return errors.NotFoundf("endpoint %q", epName)
		}
	}
	return nil
}

func (c *bindCommand) parseBindExpression(apiRoot base.APICallCloser) error {
	if c.BindExpression == "" {
		return nil
	}

	// Fetch known spaces from server
	knownSpaces, err := c.NewSpacesClient(apiRoot).ListSpaces()
	if err != nil {
		return errors.Trace(err)
	}

	knownSpaceNames := set.NewStrings()
	for _, space := range knownSpaces {
		knownSpaceNames.Add(space.Name)
	}

	// Parse expression
	bindings, err := parseBindExpr(c.BindExpression, knownSpaceNames)
	if err != nil {
		return errors.Trace(err)
	}

	c.Bindings = bindings
	return nil
}

func (c *bindCommand) checkApplicationFacadeSupport(verQuerier versionQuerier, action string, minVersion int) error {
	if verQuerier.BestFacadeVersion("Application") >= minVersion {
		return nil
	}

	suffix := "this server"
	if version, ok := verQuerier.ServerVersion(); ok {
		suffix = fmt.Sprintf("server version %s", version)
	}

	return errors.New(action + " is not supported by " + suffix)
}
