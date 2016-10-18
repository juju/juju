// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/status"
)

var usageShowControllerSummary = `
Shows detailed information of a controller.`[1:]

var usageShowControllerDetails = `
Shows extended information about a controller(s) as well as related models
and user login details.

Examples:
    juju show-controller
    juju show-controller aws google
    
See also: 
    controllers`[1:]

type showControllerCommand struct {
	modelcmd.JujuCommandBase

	out   cmd.Output
	store jujuclient.ClientStore
	api   func(controllerName string) ControllerAccessAPI

	controllerNames []string
	showPasswords   bool
}

// NewShowControllerCommand returns a command to show details of the desired controllers.
func NewShowControllerCommand() cmd.Command {
	cmd := &showControllerCommand{
		store: jujuclient.NewFileClientStore(),
	}
	return modelcmd.WrapBase(cmd)
}

// Init implements Command.Init.
func (c *showControllerCommand) Init(args []string) (err error) {
	c.controllerNames = args
	return nil
}

// Info implements Command.Info
func (c *showControllerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-controller",
		Args:    "[<controller name> ...]",
		Purpose: usageShowControllerSummary,
		Doc:     usageShowControllerDetails,
	}
}

// SetFlags implements Command.SetFlags.
func (c *showControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.JujuCommandBase.SetFlags(f)
	f.BoolVar(&c.showPasswords, "show-password", false, "Show password for logged in user")
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// ControllerAccessAPI defines a subset of the api/controller/Client API.
type ControllerAccessAPI interface {
	GetControllerAccess(user string) (permission.Access, error)
	ModelConfig() (map[string]interface{}, error)
	ModelStatus(models ...names.ModelTag) ([]base.ModelStatus, error)
	AllModels() ([]base.UserModel, error)
	Close() error
}

func (c *showControllerCommand) getAPI(controllerName string) (ControllerAccessAPI, error) {
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
func (c *showControllerCommand) Run(ctx *cmd.Context) error {
	controllerNames := c.controllerNames
	if len(controllerNames) == 0 {
		currentController, err := c.store.CurrentController()
		if errors.IsNotFound(err) {
			return errors.New("there is no active controller")
		} else if err != nil {
			return errors.Trace(err)
		}
		controllerNames = []string{currentController}
	}
	controllers := make(map[string]ShowControllerDetails)
	for _, controllerName := range controllerNames {
		one, err := c.store.ControllerByName(controllerName)
		if err != nil {
			return err
		}
		var access string
		client, err := c.getAPI(controllerName)
		if err != nil {
			return err
		}
		defer client.Close()
		accountDetails, err := c.store.AccountDetails(controllerName)
		if err != nil {
			fmt.Fprintln(ctx.Stderr, err)
			access = "(error)"
		} else {
			access = c.userAccess(client, ctx, accountDetails.User)
			one.AgentVersion = c.agentVersion(client, ctx)
		}

		var details ShowControllerDetails
		var modelStatus []base.ModelStatus
		allModels, err := client.AllModels()
		if err != nil {
			details.Errors = append(details.Errors, err.Error())
			continue
		}
		modelTags := make([]names.ModelTag, len(allModels))
		for i, m := range allModels {
			modelTags[i] = names.NewModelTag(m.UUID)
		}
		modelStatus, err = client.ModelStatus(modelTags...)
		if err != nil {
			details.Errors = append(details.Errors, err.Error())
			continue
		}
		c.convertControllerForShow(&details, controllerName, one, access, allModels, modelStatus)
		controllers[controllerName] = details
	}
	return c.out.Write(ctx, controllers)
}

func (c *showControllerCommand) userAccess(client ControllerAccessAPI, ctx *cmd.Context, user string) string {
	var access string
	userAccess, err := client.GetControllerAccess(user)
	if err == nil {
		access = string(userAccess)
	} else {
		code := params.ErrCode(err)
		if code != "" {
			access = fmt.Sprintf("(%s)", code)
		} else {
			fmt.Fprintln(ctx.Stderr, err)
			access = "(error)"
		}
	}
	return access
}

func (c *showControllerCommand) agentVersion(client ControllerAccessAPI, ctx *cmd.Context) string {
	var ver string
	mc, err := client.ModelConfig()
	if err != nil {
		code := params.ErrCode(err)
		if code != "" {
			ver = fmt.Sprintf("(%s)", code)
		} else {
			fmt.Fprintln(ctx.Stderr, err)
			ver = "(error)"
		}
		return ver
	}
	return mc["agent-version"].(string)
}

type ShowControllerDetails struct {
	// Details contains the same details that client store caches for this controller.
	Details ControllerDetails `yaml:"details,omitempty" json:"details,omitempty"`

	// Machines is a collection of all machines forming the controller cluster.
	Machines map[string]MachineDetails `yaml:"controller-machines,omitempty" json:"controller-machines,omitempty"`

	// Models is a collection of all models for this controller.
	Models map[string]ModelDetails `yaml:"models,omitempty" json:"models,omitempty"`

	// CurrentModel is the name of the current model for this controller
	CurrentModel string `yaml:"current-model,omitempty" json:"current-model,omitempty"`

	// Account is the account details for the user logged into this controller.
	Account *AccountDetails `yaml:"account,omitempty" json:"account,omitempty"`

	// Errors is a collection of errors related to accessing this controller details.
	Errors []string `yaml:"errors,omitempty" json:"errors,omitempty"`
}

// ControllerDetails holds details of a controller to show.
type ControllerDetails struct {
	// ControllerUUID is the unique ID for the controller.
	ControllerUUID string `yaml:"uuid" json:"uuid"`

	// APIEndpoints is the collection of API endpoints running in this controller.
	APIEndpoints []string `yaml:"api-endpoints,flow" json:"api-endpoints"`

	// CACert is a security certificate for this controller.
	CACert string `yaml:"ca-cert" json:"ca-cert"`

	// Cloud is the name of the cloud that this controller runs in.
	Cloud string `yaml:"cloud" json:"cloud"`

	// CloudRegion is the name of the cloud region that this controller runs in.
	CloudRegion string `yaml:"region,omitempty" json:"region,omitempty"`

	// AgentVersion is the version of the agent running on this controller.
	// AgentVersion need not always exist so we omitempty here. This struct is
	// used in both list-controller and show-controller. show-controller
	// displays the agent version where list-controller does not.
	AgentVersion string `yaml:"agent-version,omitempty" json:"agent-version,omitempty"`
}

// ModelDetails holds details of a model to show.
type MachineDetails struct {
	// ID holds the id of the machine.
	ID string `yaml:"id,omitempty" json:"id,omitempty"`

	// InstanceID holds the cloud instance id of the machine.
	InstanceID string `yaml:"instance-id,omitempty" json:"instance-id,omitempty"`

	// HAStatus holds information informing of the HA status of the machine.
	HAStatus string `yaml:"ha-status,omitempty" json:"ha-status,omitempty"`
}

// ModelDetails holds details of a model to show.
type ModelDetails struct {
	// ModelUUID holds the details of a model.
	ModelUUID string `yaml:"uuid" json:"uuid"`

	// MachineCount holds the number of machines in the model.
	MachineCount *int `yaml:"machine-count,omitempty" json:"machine-count,omitempty"`

	// CoreCount holds the number of cores across the machines in the model.
	CoreCount *int `yaml:"core-count,omitempty" json:"core-count,omitempty"`
}

// AccountDetails holds details of an account to show.
type AccountDetails struct {
	// User is the username for the account.
	User string `yaml:"user" json:"user"`

	// Access is the level of access the user has on the controller.
	Access string `yaml:"access,omitempty" json:"access,omitempty"`

	// Password is the password for the account.
	Password string `yaml:"password,omitempty" json:"password,omitempty"`
}

func (c *showControllerCommand) convertControllerForShow(
	controller *ShowControllerDetails,
	controllerName string,
	details *jujuclient.ControllerDetails,
	access string,
	allModels []base.UserModel,
	modelStatus []base.ModelStatus,
) {

	controller.Details = ControllerDetails{
		ControllerUUID: details.ControllerUUID,
		APIEndpoints:   details.APIEndpoints,
		CACert:         details.CACert,
		Cloud:          details.Cloud,
		CloudRegion:    details.CloudRegion,
		AgentVersion:   details.AgentVersion,
	}
	c.convertModelsForShow(controllerName, controller, allModels, modelStatus)
	c.convertAccountsForShow(controllerName, controller, access)
	var controllerModelUUID string
	for _, m := range allModels {
		if m.Name == bootstrap.ControllerModelName {
			controllerModelUUID = m.UUID
			break
		}
	}
	if controllerModelUUID != "" {
		var controllerModel base.ModelStatus
		found := false
		for _, m := range modelStatus {
			if m.UUID == controllerModelUUID {
				controllerModel = m
				found = true
				break
			}
		}
		if found {
			c.convertMachinesForShow(controllerName, controller, controllerModel)
		}
	}
}

func (c *showControllerCommand) convertAccountsForShow(controllerName string, controller *ShowControllerDetails, access string) {
	storeDetails, err := c.store.AccountDetails(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		controller.Errors = append(controller.Errors, err.Error())
	}
	if storeDetails == nil {
		return
	}
	details := &AccountDetails{
		User:   storeDetails.User,
		Access: access,
	}
	if c.showPasswords {
		details.Password = storeDetails.Password
	}
	controller.Account = details
}

func (c *showControllerCommand) convertModelsForShow(
	controllerName string,
	controller *ShowControllerDetails,
	models []base.UserModel,
	modelStatus []base.ModelStatus,
) {
	controller.Models = make(map[string]ModelDetails)
	for i, model := range models {
		modelDetails := ModelDetails{ModelUUID: model.UUID}
		if modelStatus[i].TotalMachineCount > 0 {
			modelDetails.MachineCount = new(int)
			*modelDetails.MachineCount = modelStatus[i].TotalMachineCount
		}
		if modelStatus[i].CoreCount > 0 {
			modelDetails.CoreCount = new(int)
			*modelDetails.CoreCount = modelStatus[i].CoreCount
		}
		controller.Models[model.Name] = modelDetails
	}
	var err error
	controller.CurrentModel, err = c.store.CurrentModel(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		controller.Errors = append(controller.Errors, err.Error())
	}
}

func (c *showControllerCommand) convertMachinesForShow(
	controllerName string,
	controller *ShowControllerDetails,
	controllerModel base.ModelStatus,
) {
	controller.Machines = make(map[string]MachineDetails)
	numControllers := 0
	for _, m := range controllerModel.Machines {
		if !m.WantsVote {
			continue
		}
		numControllers++
	}
	for _, m := range controllerModel.Machines {
		if !m.WantsVote {
			// Skip non controller machines.
			continue
		}
		instId := m.InstanceId
		if instId == "" {
			instId = "(unprovisioned)"
		}
		details := MachineDetails{InstanceID: instId}
		if numControllers > 1 {
			details.HAStatus = haStatus(m.HasVote, m.WantsVote, m.Status)
		}
		controller.Machines[m.Id] = details
	}
}

func haStatus(hasVote bool, wantsVote bool, statusStr string) string {
	if statusStr == string(status.Down) {
		return "down, lost connection"
	}
	if !wantsVote {
		return ""
	}
	if hasVote {
		return "ha-enabled"
	}
	return "ha-pending"
}
