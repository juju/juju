// Copyright 2015,2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"strings"
	"sync"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/controller"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/jujuclient"
)

var helpControllersSummary = `
Lists all controllers.`[1:]

var helpControllersDetails = `
The output format may be selected with the '--format' option. In the
default tabular output, the current controller is marked with an asterisk.

`[1:]

const helpControllersExamples = `
    juju controllers
    juju controllers --format json --output ~/tmp/controllers.json

`

// NewListControllersCommand returns a command to list registered controllers.
func NewListControllersCommand() cmd.Command {
	cmd := &listControllersCommand{
		store: jujuclient.NewFileClientStore(),
	}
	return modelcmd.WrapBase(cmd)
}

// Info implements Command.Info
func (c *listControllersCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "controllers",
		Purpose:  helpControllersSummary,
		Doc:      helpControllersDetails,
		Examples: helpControllersExamples,
		SeeAlso: []string{
			"models",
			"show-controller",
		},
		Aliases: []string{"list-controllers"},
	})
}

// Init implements Command.
func (c *listControllersCommand) Init(args []string) error {
	if c.managed {
		return cmd.ErrCommandMissing
	}

	return cmd.CheckEmpty(args)
}

// SetFlags implements Command.SetFlags.
func (c *listControllersCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.refresh, "refresh", false, "Connect to each controller to download the latest details")
	f.BoolVar(&c.managed, "managed", false, "Show controllers managed by JAAS")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatControllersListTabular,
	})
}

// SetClientStore implements Command.SetClientStore.
func (c *listControllersCommand) SetClientStore(store jujuclient.ClientStore) {
	c.store = store
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
	if len(controllers) == 0 && c.out.Name() == "tabular" {
		return errors.Trace(modelcmd.ErrNoControllersDefined)
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
	currentController, err := modelcmd.DetermineCurrentController(c.store)
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
	allModels, err := client.AllModels()
	if err != nil {
		return err
	}
	// Update client store.
	if err := c.SetControllerModels(c.store, controllerName, allModels); err != nil {
		return errors.Trace(err)
	}

	var controllerModelUUID string
	modelTags := make([]names.ModelTag, len(allModels))
	for i, m := range allModels {
		modelTags[i] = names.NewModelTag(m.UUID)
		if m.Name == bootstrap.ControllerModelName {
			controllerModelUUID = m.UUID
		}
	}
	modelStatus, err := client.ModelStatus(modelTags...)
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

	machineCount := 0
	for _, s := range modelStatus {
		if s.Error != nil {
			if errors.IsNotFound(s.Error) {
				// This most likely occurred because a model was
				// destroyed half-way through the call.
				continue
			}
			return errors.Trace(s.Error)
		}
		machineCount += s.TotalMachineCount
	}
	details.MachineCount = &machineCount
	details.ActiveControllerMachineCount, details.ControllerMachineCount = ControllerMachineCounts(controllerModelUUID, modelStatus)
	return c.store.UpdateController(controllerName, *details)
}

func ControllerMachineCounts(controllerModelUUID string, modelStatusResults []base.ModelStatus) (activeCount, totalCount int) {
	for _, s := range modelStatusResults {
		if s.Error != nil {
			// This most likely occurred because a model was
			// destroyed half-way through the call.
			continue
		}
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
	modelcmd.CommandBase

	out     cmd.Output
	store   jujuclient.ClientStore
	api     func(controllerName string) ControllerAccessAPI
	refresh bool
	mu      sync.Mutex
	// managed is useful when JAAS is available and lists
	// controllers managed by JAAS.
	managed bool
}
