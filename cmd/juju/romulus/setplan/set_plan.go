// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The setplan package contains the implementation of the juju set-plan
// command.
package setplan

import (
	"encoding/json"
	"net/url"

	"github.com/juju/juju/core/model"
	"github.com/juju/names/v4"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	api "github.com/juju/romulus/api/plan"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/application"
	jujucmd "github.com/juju/juju/cmd"
	rcmd "github.com/juju/juju/cmd/juju/romulus"
	"github.com/juju/juju/cmd/modelcmd"
)

// authorizationClient defines the interface of an api client that
// the command uses to create an authorization macaroon.
type authorizationClient interface {
	// Authorize returns the authorization macaroon for the specified environment,
	// charm url, application name and plan.
	Authorize(environmentUUID, charmURL, applicationName, plan string, visitWebPage func(*url.URL) error) (*macaroon.Macaroon, error)
}

var newAuthorizationClient = func(options ...api.ClientOption) (authorizationClient, error) {
	return api.NewAuthorizationClient(options...)
}

// NewSetPlanCommand returns a new command that is used to set metric credentials for a
// deployed application.
func NewSetPlanCommand() cmd.Command {
	return modelcmd.Wrap(&setPlanCommand{})
}

// setPlanCommand is a command-line tool for setting
// Application.MetricCredential for development & demonstration purposes.
type setPlanCommand struct {
	modelcmd.ModelCommandBase

	Application string
	Plan        string
}

// Info implements cmd.Command.
func (c *setPlanCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "set-plan",
		Args:    "<application name> <plan>",
		Purpose: "Set the plan for an application.",
		Doc: `
Set the plan for the deployed application, effective immediately.

The specified plan name must be a valid plan that is offered for this
particular charm. Use "juju list-plans <charm>" for more information.

Examples:
    juju set-plan myapp example/uptime
`,
	})
}

// Init implements cmd.Command.
func (c *setPlanCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.New("need to specify application name and plan url")
	}

	applicationName := args[0]
	if !names.IsValidApplication(applicationName) {
		return errors.Errorf("invalid application name %q", applicationName)
	}

	c.Plan = args[1]
	c.Application = applicationName

	return c.ModelCommandBase.Init(args[2:])
}

func (c *setPlanCommand) requestMetricCredentials(client *application.Client, ctx *cmd.Context) ([]byte, error) {
	charmURL, err := client.GetCharmURL(model.GenerationMaster, c.Application)
	if err != nil {
		return nil, errors.Trace(err)
	}

	hc, err := c.BakeryClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	apiRoot, err := rcmd.GetMeteringURLForModelCmd(&c.ModelCommandBase)
	if err != nil {
		return nil, errors.Trace(err)
	}
	authClient, err := newAuthorizationClient(api.APIRoot(apiRoot), api.HTTPClient(hc))
	if err != nil {
		return nil, errors.Trace(err)
	}
	m, err := authClient.Authorize(client.ModelUUID(), charmURL.String(), c.Application, c.Plan, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ms := macaroon.Slice{m}
	return json.Marshal(ms)
}

// Run implements cmd.Command.
func (c *setPlanCommand) Run(ctx *cmd.Context) error {
	root, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	client := application.NewClient(root)
	credentials, err := c.requestMetricCredentials(client, ctx)
	if err != nil {
		return errors.Trace(err)
	}
	err = client.SetMetricCredentials(c.Application, credentials)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
