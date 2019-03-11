// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"sync"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/permission"
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
	modelcmd.CommandBase

	out   cmd.Output
	store jujuclient.ClientStore
	mu    sync.Mutex
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
	return jujucmd.Info(&cmd.Info{
		Name:    "show-controller",
		Args:    "[<controller name> ...]",
		Purpose: usageShowControllerSummary,
		Doc:     usageShowControllerDetails,
	})
}

// SetFlags implements Command.SetFlags.
func (c *showControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
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
	MongoVersion() (string, error)
	IdentityProviderURL() (string, error)
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
		currentController, err := modelcmd.DetermineCurrentController(c.store)
		if errors.IsNotFound(err) {
			return errors.New("there is no active controller")
		} else if err != nil {
			return errors.Trace(err)
		}
		controllerNames = []string{currentController}
	}
	controllers := make(map[string]ShowControllerDetails)
	c.mu.Lock()
	defer c.mu.Unlock()
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

		var (
			details      ShowControllerDetails
			allModels    []base.UserModel
			mongoVersion string
		)

		// NOTE: this user may have been granted AddModelAccess which
		// should allow them to list only the models they have access to.
		// However, the code that grants permissions currently uses an
		// escape hatch (to be removed in juju 3) that actually grants
		// controller cloud access instead of controller access.
		//
		// The side-effect to this is that the userAccess() call above
		// will return LoginAccess even if the user has been granted
		// AddModelAccess causing the calls in the block below to fail
		// with a permission error. As a workaround, unless the user
		// has Superuser access we default to an empty model list which
		// allows us to display non-model controller details.
		if permission.Access(access).EqualOrGreaterControllerAccessThan(permission.SuperuserAccess) {
			if allModels, err = client.AllModels(); err != nil {
				details.Errors = append(details.Errors, err.Error())
				continue
			}
			// Update client store.
			if err := c.SetControllerModels(c.store, controllerName, allModels); err != nil {
				details.Errors = append(details.Errors, err.Error())
				continue
			}

			// Fetch mongoVersion if the apiserver supports it
			mongoVersion, err = client.MongoVersion()
			if err != nil && !errors.IsNotSupported(err) {
				details.Errors = append(details.Errors, err.Error())
				continue
			}
		}

		// Fetch identityURL if the apiserver supports it
		identityURL, err := client.IdentityProviderURL()
		if err != nil && !errors.IsNotSupported(err) {
			details.Errors = append(details.Errors, err.Error())
			continue
		}

		modelTags := make([]names.ModelTag, len(allModels))
		var controllerModelUUID string
		for i, m := range allModels {
			modelTags[i] = names.NewModelTag(m.UUID)
			if m.Name == bootstrap.ControllerModelName {
				controllerModelUUID = m.UUID
			}
		}
		modelStatusResults, err := client.ModelStatus(modelTags...)
		if err != nil {
			details.Errors = append(details.Errors, err.Error())
			continue
		}

		c.convertControllerForShow(&details, controllerName, one, access, allModels, modelStatusResults, mongoVersion, identityURL)
		controllers[controllerName] = details
		machineCount := 0
		for _, r := range modelStatusResults {
			if r.Error != nil {
				if !errors.IsNotFound(r.Error) {
					details.Errors = append(details.Errors, r.Error.Error())
				}
				continue
			}
			machineCount += r.TotalMachineCount
		}
		one.MachineCount = &machineCount
		one.ActiveControllerMachineCount, one.ControllerMachineCount = ControllerMachineCounts(controllerModelUUID, modelStatusResults)
		err = c.store.UpdateController(controllerName, *one)
		if err != nil {
			details.Errors = append(details.Errors, err.Error())
		}
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
	// TODO(anastasiamac 2018-08-10) This is a deprecated property, see lp#1596607.
	// It was added for backward compatibility, lp#1786061, to be removed for Juju 3.
	OldControllerUUID string `yaml:"uuid" json:"-"`

	// ControllerUUID is the unique ID for the controller.
	ControllerUUID string `yaml:"controller-uuid" json:"uuid"`

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

	// MongoVersion is the version of the mongo server running on this
	// controller.
	MongoVersion string `yaml:"mongo-version,omitempty" json:"mongo-version,omitempty"`

	// IdentityURL contails the address of an external identity provider
	// if one has been configured for this controller.
	IdentityURL string `yaml:"identity-url,omitempty" json:"identity-url,omitempty"`
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
	// TODO(anastasiamac 2018-08-10) This is a deprecated property, see lp#1596607.
	// It was added for backward compatibility, lp#1786061, to be removed for Juju 3.
	OldModelUUID string `yaml:"uuid" json:"-"`

	// ModelUUID holds the details of a model.
	ModelUUID string `yaml:"model-uuid" json:"uuid"`

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
	modelStatusResults []base.ModelStatus,
	mongoVersion string,
	identityURL string,
) {

	controller.Details = ControllerDetails{
		ControllerUUID:    details.ControllerUUID,
		OldControllerUUID: details.ControllerUUID,
		APIEndpoints:      details.APIEndpoints,
		CACert:            details.CACert,
		Cloud:             details.Cloud,
		CloudRegion:       details.CloudRegion,
		AgentVersion:      details.AgentVersion,
		MongoVersion:      mongoVersion,
		IdentityURL:       identityURL,
	}
	c.convertModelsForShow(controllerName, controller, allModels, modelStatusResults)
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
		for _, m := range modelStatusResults {
			if m.Error != nil {
				// This most likely occurred because a model was
				// destroyed half-way through the call.
				continue
			}
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
		modelDetails := ModelDetails{ModelUUID: model.UUID, OldModelUUID: model.UUID}
		result := modelStatus[i]
		if result.Error != nil {
			if !errors.IsNotFound(result.Error) {
				controller.Errors = append(controller.Errors, errors.Annotatef(result.Error, "model uuid %v", model.UUID).Error())
			}
		} else {
			if result.TotalMachineCount > 0 {
				modelDetails.MachineCount = new(int)
				*modelDetails.MachineCount = result.TotalMachineCount
			}
			if result.CoreCount > 0 {
				modelDetails.CoreCount = new(int)
				*modelDetails.CoreCount = result.CoreCount
			}
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
