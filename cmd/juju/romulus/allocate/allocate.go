// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package allocate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/utils"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	api "github.com/juju/romulus/api/budget"
)

var budgetWithLimitRe = regexp.MustCompile(`^[a-zA-Z0-9\-]+:[0-9]+$`)

type allocateCommand struct {
	modelcmd.ModelCommandBase
	api       apiClient
	Budget    string
	ModelUUID string
	Services  []string
	Limit     string
}

// NewAllocateCommand returns a new allocateCommand
func NewAllocateCommand() modelcmd.ModelCommand {
	return &allocateCommand{}
}

const doc = `
Allocate budget for the specified applications, replacing any prior allocations
made for the specified applications.

Examples:
    # Assigns application "db" to an allocation on budget "somebudget" with
    # the limit "42".
    juju allocate somebudget:42 db

    # Application names assume the current selected model, unless otherwise
    # specified with:
    juju allocate -m [<controller name:]<model name> ...

    # Models may also be referenced by UUID when necessary:
     juju allocate --model-uuid <uuid> ...
`

// SetFlags implements cmd.Command.SetFlags.
func (c *allocateCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.ModelUUID, "model-uuid", "", "Model UUID of allocation")
}

// Info implements cmd.Command.Info.
func (c *allocateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "allocate",
		Args:    "<budget>:<value> <application> [<application2> ...]",
		Purpose: "Allocate budget to applications.",
		Doc:     doc,
	}
}

// Init implements cmd.Command.Init.
func (c *allocateCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.New("budget and application name required")
	}
	budgetWithLimit := args[0]
	var err error
	c.Budget, c.Limit, err = parseBudgetWithLimit(budgetWithLimit)
	if err != nil {
		return errors.Annotate(err, `expected args in the form "budget:limit [application ...]"`)
	}
	if c.ModelUUID == "" {
		c.ModelUUID, err = c.modelUUID()
		if err != nil {
			return err
		}
	} else {
		if !utils.IsValidUUIDString(c.ModelUUID) {
			return errors.NotValidf("model UUID %q", c.ModelUUID)
		}
	}

	c.Services = args[1:]
	return nil
}

// Run implements cmd.Command.Run and has most of the logic for the run command.
func (c *allocateCommand) Run(ctx *cmd.Context) error {
	client, err := c.BakeryClient()
	if err != nil {
		return errors.Annotate(err, "failed to create an http client")
	}
	api, err := c.newAPIClient(client)
	if err != nil {
		return errors.Annotate(err, "failed to create an api client")
	}
	resp, err := api.CreateAllocation(c.Budget, c.Limit, c.ModelUUID, c.Services)
	if err != nil {
		return errors.Annotate(err, "failed to create allocation")
	}
	fmt.Fprintln(ctx.Stdout, resp)
	return nil
}

func (c *allocateCommand) modelUUID() (string, error) {
	model, err := c.ClientStore().ModelByName(c.ControllerName(), c.ModelName())
	if err != nil {
		return "", errors.Trace(err)
	}
	return model.ModelUUID, nil
}

func parseBudgetWithLimit(bl string) (string, string, error) {
	if !budgetWithLimitRe.MatchString(bl) {
		return "", "", errors.New("invalid budget specification, expecting <budget>:<limit>")
	}
	parts := strings.Split(bl, ":")
	return parts[0], parts[1], nil
}

func (c *allocateCommand) newAPIClient(bakery *httpbakery.Client) (apiClient, error) {
	if c.api != nil {
		return c.api, nil
	}
	c.api = api.NewClient(bakery)
	return c.api, nil
}

type apiClient interface {
	CreateAllocation(string, string, string, []string) (string, error)
}
