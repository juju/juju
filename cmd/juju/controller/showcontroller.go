// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"sync"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/controller"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/pki"
	"github.com/juju/juju/rpc/params"
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
	command := &showControllerCommand{
		store: jujuclient.NewFileClientStore(),
	}
	return modelcmd.WrapBase(command)
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

// SetClientStore implements Command.SetClientStore.
func (c *showControllerCommand) SetClientStore(store jujuclient.ClientStore) {
	c.store = store
}

// ControllerAccessAPI defines a subset of the api/controller/Client API.
type ControllerAccessAPI interface {
	BestAPIVersion() int
	GetControllerAccess(user string) (permission.Access, error)
	ModelConfig() (map[string]interface{}, error)
	ModelStatus(models ...names.ModelTag) ([]base.ModelStatus, error)
	AllModels() ([]base.UserModel, error)
	MongoVersion() (string, error)
	IdentityProviderURL() (string, error)
	ControllerVersion() (controller.ControllerVersion, error)
	ControllerNodes() ([]controller.ControllerNode, error)
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

		var (
			details           ShowControllerDetails
			allModels         []base.UserModel
			mongoVersion      string
			controllerVersion string
			agentGitCommit    string
		)

		accountDetails, err := c.store.AccountDetails(controllerName)
		if err != nil {
			fmt.Fprintln(ctx.Stderr, err)
			access = "(error)"
		} else {
			access = c.userAccess(client, ctx, accountDetails.User)
			controllerVersion = c.controllerModelVersion(client, ctx)
		}

		ver, err := client.ControllerVersion()
		if err != nil && !errors.IsNotSupported(err) {
			details.Errors = append(details.Errors, err.Error())
			agentGitCommit = "(error)"
		} else if !errors.IsNotSupported(err) {
			one.AgentVersion = ver.Version
			agentGitCommit = ver.GitCommit
		}

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
			} else {
				// Update client store.
				if err := c.SetControllerModels(c.store, controllerName, allModels); err != nil {
					details.Errors = append(details.Errors, err.Error())
				}
			}
			// Fetch mongoVersion if the apiserver supports it
			mongoVersion, err = client.MongoVersion()
			if err != nil && !errors.IsNotSupported(err) {
				details.Errors = append(details.Errors, err.Error())
				mongoVersion = "(error)"
			}
		}

		// Fetch identityURL if the apiserver supports it
		identityURL, err := client.IdentityProviderURL()
		if err != nil && !errors.IsNotSupported(err) {
			details.Errors = append(details.Errors, err.Error())
			identityURL = "(error)"
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
		}

		// Update controller in local store.
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

		// Only update the local controller store if no errors were encountered.
		if len(details.Errors) == 0 {
			err = c.store.UpdateController(controllerName, *one)
			if err != nil {
				details.Errors = append(details.Errors, err.Error())
			}
		}
		var controllerNodes []controller.ControllerNode
		if client.BestAPIVersion() > 11 {
			controllerNodes, err = client.ControllerNodes()
			if err != nil {
				details.Errors = append(details.Errors, err.Error())
			}
		} else {
			controllerNodes = c.oldStyleControllerNodes(modelStatusResults, allModels)
		}

		c.convertControllerForShow(&details, controllerName, one, access, allModels, controllerNodes,
			modelStatusResults, mongoVersion, controllerVersion, agentGitCommit, identityURL)
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

func (c *showControllerCommand) controllerModelVersion(client ControllerAccessAPI, ctx *cmd.Context) string {
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

func (c *showControllerCommand) oldStyleControllerNodes(
	modelStatusResults []base.ModelStatus,
	allModels []base.UserModel,
) []controller.ControllerNode {
	var controllerModelUUID string
	for _, model := range allModels {
		if model.Name == bootstrap.ControllerModelName {
			controllerModelUUID = model.UUID
			break
		}
	}
	if controllerModelUUID == "" {
		return []controller.ControllerNode{}
	}
	var controllerModel base.ModelStatus
	for _, model := range modelStatusResults {
		if model.Error == nil && model.UUID == controllerModelUUID {
			controllerModel = model
			break
		}
	}
	if controllerModel.ModelType == model.CAAS {
		return []controller.ControllerNode{}
	}
	nodes := make([]controller.ControllerNode, 0, len(controllerModel.Machines))
	for _, machine := range controllerModel.Machines {
		if !machine.WantsVote {
			// Skip non controller machines.
			continue
		}
		controller := controller.ControllerNode{
			Id:         machine.Id,
			InstanceId: machine.InstanceId,
			HasVote:    machine.HasVote,
			WantsVote:  machine.WantsVote,
			Status:     machine.Status,
		}
		if machine.HAPrimary != nil && *machine.HAPrimary {
			controller.IsPrimary = true
		}
		nodes = append(nodes, controller)
	}
	return nodes
}

type ShowControllerDetails struct {
	// Details contains the same details that client store caches for this controller.
	Details ControllerDetails `yaml:"details,omitempty" json:"details,omitempty"`

	// Nodes is a collection of all nodes (machines/pods) forming the controller cluster.
	Nodes map[string]MachineDetails `yaml:"controller-nodes,omitempty" json:"controller-nodes,omitempty"`

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

	// Cloud is the name of the cloud that this controller runs in.
	Cloud string `yaml:"cloud" json:"cloud"`

	// CloudRegion is the name of the cloud region that this controller runs in.
	CloudRegion string `yaml:"region,omitempty" json:"region,omitempty"`

	// AgentVersion is the version of the agent running on this controller.
	// AgentVersion need not always exist so we omitempty here. This struct is
	// used in both list-controller and show-controller. show-controller
	// displays the agent version where list-controller does not.
	AgentVersion string `yaml:"agent-version,omitempty" json:"agent-version,omitempty"`

	// AgentGitCommit is the git commit hash used to build the controller binary.
	AgentGitCommit string `yaml:"agent-git-commit,omitempty" json:"agent-git-commit,omitempty"`

	// ControllerModelVersion is the version in the controller model config state.
	ControllerModelVersion string `yaml:"controller-model-version,omitempty" json:"controller-model-version,omitempty"`

	// MongoVersion is the version of the mongo server running on this
	// controller.
	MongoVersion string `yaml:"mongo-version,omitempty" json:"mongo-version,omitempty"`

	// IdentityURL contails the address of an external identity provider
	// if one has been configured for this controller.
	IdentityURL string `yaml:"identity-url,omitempty" json:"identity-url,omitempty"`

	// SHA-256 fingerprint of the CA cert
	CAFingerprint string `yaml:"ca-fingerprint,omitempty" json:"ca-fingerprint,omitempty"`

	// CACert is a security certificate for this controller.
	CACert string `yaml:"ca-cert" json:"ca-cert"`
}

// ModelDetails holds details of a model to show.
type MachineDetails struct {
	// ID holds the id of the machine.
	ID string `yaml:"id,omitempty" json:"id,omitempty"`

	// InstanceID holds the cloud instance id of the machine.
	InstanceID string `yaml:"instance-id,omitempty" json:"instance-id,omitempty"`

	// Life is the lifecycle state of the machine
	Life string `yaml:"life,omitempty" json:"life,omitempty"`

	// HAStatus holds information informing of the HA status of the machine.
	HAStatus string `yaml:"ha-status,omitempty" json:"ha-status,omitempty"`

	// HAPrimary is set to true for a primary controller machine in HA.
	HAPrimary bool `yaml:"ha-primary,omitempty" json:"ha-primary,omitempty"`
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

	// UnitCount holds the number of units in the model.
	UnitCount *int `yaml:"unit-count,omitempty" json:"unit-count,omitempty"`
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
	controllerNodes []controller.ControllerNode,
	modelStatusResults []base.ModelStatus,
	mongoVersion string,
	controllerVersion string,
	agentGitCommit string,
	identityURL string,
) {
	// CA cert will always be valid so no need to check for errors here
	caFingerprint, _, _ := pki.Fingerprint([]byte(details.CACert))

	controller.Details = ControllerDetails{
		ControllerUUID:         details.ControllerUUID,
		OldControllerUUID:      details.ControllerUUID,
		APIEndpoints:           details.APIEndpoints,
		CACert:                 details.CACert,
		CAFingerprint:          caFingerprint,
		Cloud:                  details.Cloud,
		CloudRegion:            details.CloudRegion,
		AgentVersion:           details.AgentVersion,
		AgentGitCommit:         agentGitCommit,
		ControllerModelVersion: controllerVersion,
		MongoVersion:           mongoVersion,
		IdentityURL:            identityURL,
	}
	c.convertModelsForShow(controllerName, controller, allModels, modelStatusResults)
	c.convertAccountsForShow(controllerName, controller, access)
	c.convertMachinesForShow(controller, controllerNodes)
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
	if len(models) != len(modelStatus) {
		controller.Errors = append(controller.Errors, "model status incomplete")
	}
	for i, m := range models {
		if i >= len(modelStatus) {
			continue
		}
		modelDetails := ModelDetails{ModelUUID: m.UUID, OldModelUUID: m.UUID}
		result := modelStatus[i]
		if result.Error != nil {
			if !errors.IsNotFound(result.Error) {
				controller.Errors = append(controller.Errors, errors.Annotatef(result.Error, "model uuid %v", m.UUID).Error())
			}
		} else {
			if m.Type == model.CAAS {
				if result.UnitCount > 0 {
					modelDetails.UnitCount = new(int)
					*modelDetails.UnitCount = result.UnitCount
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
		}
		controller.Models[m.Name] = modelDetails
	}
	var err error
	controller.CurrentModel, err = c.store.CurrentModel(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		controller.Errors = append(controller.Errors, err.Error())
	}
}

func (c *showControllerCommand) convertMachinesForShow(
	controller *ShowControllerDetails,
	controllerNodes []controller.ControllerNode,
) {
	nodes := make(map[string]MachineDetails)
	for _, controllerNode := range controllerNodes {
		details := MachineDetails{
			InstanceID: controllerNode.InstanceId,
			Life:       controllerNode.Life,
		}
		if len(controllerNodes) > 1 {
			details.HAPrimary = controllerNode.IsPrimary
			details.HAStatus = haStatus(controllerNode.HasVote, controllerNode.WantsVote, controllerNode.Status)
		}
		nodes[controllerNode.Id] = details
	}
	controller.Nodes = nodes
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
