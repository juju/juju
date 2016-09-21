// Copyright 2015,2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"strings"
	"sync"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/status"
)

var helpControllersSummary = `
Lists all controllers.`[1:]

var helpControllersDetails = `
The output format may be selected with the '--format' option. In the
default tabular output, the current controller is marked with an asterisk.

Examples:
    juju controllers
    juju controllers --format json --output ~/tmp/controllers.json

See also:
    models
    show-controller`[1:]

// NewListControllersCommand returns a command to list registered controllers.
func NewListControllersCommand() cmd.Command {
	cmd := &listControllersCommand{
		store: jujuclient.NewFileClientStore(),
	}
	return modelcmd.WrapBase(cmd)
}

// Info implements Command.Info
func (c *listControllersCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "controllers",
		Purpose: helpControllersSummary,
		Doc:     helpControllersDetails,
		Aliases: []string{"list-controllers"},
	}
}

// SetFlags implements Command.SetFlags.
func (c *listControllersCommand) SetFlags(f *gnuflag.FlagSet) {
	c.JujuCommandBase.SetFlags(f)
	f.BoolVar(&c.refresh, "refresh", false, "Connect to each controller to download the latest details")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatControllersListTabular,
	})
}

func (c *listControllersCommand) getAPI(controllerName string) (ControllerAccessAPI, error) {
	if c.api != nil {
		return c.api(controllerName), nil
	}
	api, err := c.NewAPIRoot(c.store, controllerName, "")
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	return controller.NewClient(api), nil
}

// Run implements Command.Run
func (c *listControllersCommand) Run(ctx *cmd.Context) error {
	controllers, err := c.store.AllControllers()
	if err != nil {
		return errors.Annotate(err, "failed to list controllers")
	}
	if c.refresh && len(controllers) > 0 {
		var wg sync.WaitGroup
		wg.Add(len(controllers))
		for controllerName := range controllers {
			name := controllerName
			go func() {
				defer wg.Done()
				client, err := c.getAPI(name)
				if err != nil {
					fmt.Fprintf(ctx.GetStderr(), "error connecting to api for %q: %v\n", name, err)
					return
				}
				defer client.Close()
				if err := c.refreshControllerDetails(client, name); err != nil {
					fmt.Fprintf(ctx.GetStderr(), "error updating cached details for %q: %v\n", name, err)
				}
			}()
		}
		wg.Wait()
		// Reload controller details
		controllers, err = c.store.AllControllers()
		if err != nil {
			return errors.Annotate(err, "failed to list controllers")
		}
	}
	details, errs := c.convertControllerDetails(controllers)
	if len(errs) > 0 {
		fmt.Fprintln(ctx.Stderr, strings.Join(errs, "\n"))
	}
	currentController, err := c.store.CurrentController()
	if errors.IsNotFound(err) {
		currentController = ""
	} else if err != nil {
		return errors.Annotate(err, "getting current controller")
	}
	controllerSet := ControllerSet{
		Controllers:       details,
		CurrentController: currentController,
	}
	return c.out.Write(ctx, controllerSet)
}

func (c *listControllersCommand) refreshControllerDetails(client ControllerAccessAPI, controllerName string) error {
	// First, get all the models the user can see, and their details.
	var modelStatus []base.ModelStatus
	allModels, err := client.AllModels()
	if err != nil {
		return err
	}
	var controllerModelUUID string
	modelTags := make([]names.ModelTag, len(allModels))
	for i, m := range allModels {
		modelTags[i] = names.NewModelTag(m.UUID)
		if m.Name == bootstrap.ControllerModelName {
			controllerModelUUID = m.UUID
		}
	}
	modelStatus, err = client.ModelStatus(modelTags...)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// Use the model information to update the cached controller details.
	details, err := c.store.ControllerByName(controllerName)
	if err != nil {
		return err
	}

	modelCount := len(allModels)
	details.ModelCount = &modelCount
	machineCount := 0
	for _, s := range modelStatus {
		machineCount += s.TotalMachineCount
	}
	details.MachineCount = &machineCount
	details.ActiveControllerMachineCount, details.ControllerMachineCount = controllerMachineCounts(controllerModelUUID, modelStatus)
	return c.store.UpdateController(controllerName, *details)
}

func controllerMachineCounts(controllerModelUUID string, modelStatus []base.ModelStatus) (activeCount, totalCount int) {
	for _, s := range modelStatus {
		if s.UUID != controllerModelUUID {
			continue
		}
		for _, m := range s.Machines {
			if !m.WantsVote {
				continue
			}
			totalCount++
			if m.Status != string(status.Down) && m.HasVote {
				activeCount++
			}
		}
	}
	return activeCount, totalCount
}

type listControllersCommand struct {
	modelcmd.JujuCommandBase

	out     cmd.Output
	store   jujuclient.ClientStore
	api     func(controllerName string) ControllerAccessAPI
	refresh bool
	mu      sync.Mutex
}
