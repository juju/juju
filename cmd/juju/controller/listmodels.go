// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/juju/ansiterm"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
)

// NewListModelsCommand returns a command to list models.
func NewListModelsCommand() cmd.Command {
	return modelcmd.WrapController(&modelsCommand{})
}

// ModelManagerAPI defines the methods on the model manager API that
// the models command calls.
type ModelManagerAPI interface {
	Close() error
	ListModels(user string) ([]base.UserModel, error)
	ListModelSummaries(user string, all bool) ([]base.UserModelSummary, error)
	ModelInfo([]names.ModelTag) ([]params.ModelInfoResult, error)
	BestAPIVersion() int
}

// modelsCommand returns the list of all the models the
// current user can access on the current controller.
type modelsCommand struct {
	modelcmd.ControllerCommandBase
	out          cmd.Output
	all          bool
	loggedInUser string
	user         string
	listUUID     bool
	exactTime    bool
	modelAPI     ModelManagerAPI
	sysAPI       ModelsSysAPI

	runVars modelsRunValues
}

// Info implements Command.Info
func (c *modelsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "models",
		Purpose: "Lists models a user can access on a controller.",
		Doc:     listModelsDoc,
		Aliases: []string{"list-models"},
	}
}

// SetFlags implements Command.SetFlags.
func (c *modelsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	f.StringVar(&c.user, "user", "", "The user to list models for (administrative users only)")
	f.BoolVar(&c.all, "all", false, "Lists all models, regardless of user accessibility (administrative users only)")
	f.BoolVar(&c.listUUID, "uuid", false, "Display UUID for models")
	f.BoolVar(&c.exactTime, "exact-time", false, "Use full timestamps")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatTabular,
	})
}

// Run implements Command.Run
func (c *modelsCommand) Run(ctx *cmd.Context) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		ctx.Infof(err.Error())
		return errors.Trace(err)
	}
	accountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		ctx.Infof(err.Error())
		return err
	}
	c.loggedInUser = accountDetails.User

	if c.user == "" {
		c.user = accountDetails.User
	}
	if !names.IsValidUser(c.user) {
		err := errors.NotValidf("user %q", c.user)
		ctx.Infof(err.Error())
		return err
	}

	c.runVars = modelsRunValues{
		currentUser:    names.NewUserTag(c.user),
		controllerName: controllerName,
	}
	// TODO(perrito666) 2016-05-02 lp:1558657
	now := time.Now()

	modelmanagerAPI, err := c.getModelManagerAPI()
	if err != nil {
		ctx.Infof(err.Error())
		return errors.Trace(err)
	}
	defer modelmanagerAPI.Close()

	haveModels := false
	if modelmanagerAPI.BestAPIVersion() > 3 {
		haveModels, err = c.getModelSummaries(ctx, modelmanagerAPI, now)
		if err != nil {
			// This is needed to provide a consistent behavior with previous
			// 'models' implementation.
			err = errors.Annotate(err, "cannot list models")
		}
	} else {
		haveModels, err = c.oldModelsCommandBehaviour(ctx, modelmanagerAPI, now)
	}
	if err != nil {
		ctx.Infof(err.Error())
		return err
	}
	if !haveModels && c.out.Name() == "tabular" {
		// When the output is tabular, we inform the user when there
		// are no models available, and tell them how to go about
		// creating or granting access to them.
		fmt.Fprintln(ctx.Stderr, noModelsMessage)
	}
	return nil
}

func (c *modelsCommand) currentModelName() (qualified, name string) {
	current, err := c.ClientStore().CurrentModel(c.runVars.controllerName)
	if err == nil {
		qualified, name = current, current
		if c.user != "" {
			unqualifiedModelName, owner, err := jujuclient.SplitModelName(current)
			if err == nil {
				// If current model's owner is this user, un-qualify model name.
				name = common.OwnerQualifiedModelName(
					unqualifiedModelName, owner, c.runVars.currentUser,
				)
			}
		}
	}
	return
}

func (c *modelsCommand) getModelManagerAPI() (ModelManagerAPI, error) {
	if c.modelAPI != nil {
		return c.modelAPI, nil
	}
	return c.NewModelManagerAPIClient()
}

func (c *modelsCommand) getModelSummaries(ctx *cmd.Context, client ModelManagerAPI, now time.Time) (bool, error) {
	results, err := client.ListModelSummaries(c.user, c.all)
	if err != nil {
		return false, errors.Trace(err)
	}
	summaries := []ModelSummary{}
	modelsToStore := map[string]jujuclient.ModelDetails{}
	for _, result := range results {
		// Since we do not want to throw away all results if we have an
		// an issue with a model, we will display errors in Stderr
		// and will continue processing the rest.
		if result.Error != nil {
			ctx.Infof(result.Error.Error())
			continue
		}
		model, err := c.modelSummaryFromParams(result, now)
		if err != nil {
			ctx.Infof(err.Error())
			continue
		}
		model.ControllerName = c.runVars.controllerName
		summaries = append(summaries, model)
		modelsToStore[model.Name] = jujuclient.ModelDetails{ModelUUID: model.UUID, ModelType: model.Type}
	}
	found := len(summaries) > 0

	if err := c.ClientStore().SetModels(c.runVars.controllerName, modelsToStore); err != nil {
		return found, errors.Trace(err)
	}

	// Identifying current model has to be done after models in client store have been updated
	// since that call determines/updates current model information.
	modelSummarySet := ModelSummarySet{Models: summaries}
	modelSummarySet.CurrentModelQualified, modelSummarySet.CurrentModel = c.currentModelName()
	if err := c.out.Write(ctx, modelSummarySet); err != nil {
		return found, err
	}
	return found, err
}

// ModelSummarySet contains the set of summaries for models.
type ModelSummarySet struct {
	Models []ModelSummary `yaml:"models" json:"models"`

	// CurrentModel is the name of the current model, qualified for the
	// user for which we're listing models. i.e. for the user admin,
	// and the model admin/foo, this field will contain "foo"; for
	// bob and the same model, the field will contain "admin/foo".
	CurrentModel string `yaml:"current-model,omitempty" json:"current-model,omitempty"`

	// CurrentModelQualified is the fully qualified name for the current
	// model, i.e. having the format $owner/$model.
	CurrentModelQualified string `yaml:"-" json:"-"`
}

// ModelSummary contains a summary of some information about a model.
type ModelSummary struct {
	// Name is a fully qualified model name, i.e. having the format $owner/$model.
	Name string `json:"name" yaml:"name"`

	// ShortName is un-qualified model name.
	ShortName string          `json:"short-name" yaml:"short-name"`
	UUID      string          `json:"model-uuid" yaml:"model-uuid"`
	Type      model.ModelType `json:"model-type" yaml:"model-type"`

	ControllerUUID     string                  `json:"controller-uuid" yaml:"controller-uuid"`
	ControllerName     string                  `json:"controller-name" yaml:"controller-name"`
	Owner              string                  `json:"owner" yaml:"owner"`
	Cloud              string                  `json:"cloud" yaml:"cloud"`
	CloudRegion        string                  `json:"region,omitempty" yaml:"region,omitempty"`
	CloudCredential    *common.ModelCredential `json:"credential,omitempty" yaml:"credential,omitempty"`
	ProviderType       string                  `json:"type,omitempty" yaml:"type,omitempty"`
	Life               string                  `json:"life" yaml:"life"`
	Status             *common.ModelStatus     `json:"status,omitempty" yaml:"status,omitempty"`
	UserAccess         string                  `yaml:"access" json:"access"`
	UserLastConnection string                  `yaml:"last-connection" json:"last-connection"`

	// Counts is the map of different counts where key is the entity that was counted
	// and value is the number, for e.g. {"machines":10,"cores":3}.
	Counts       map[string]int64 `json:"-" yaml:"-"`
	SLA          string           `json:"sla,omitempty" yaml:"sla,omitempty"`
	SLAOwner     string           `json:"sla-owner,omitempty" yaml:"sla-owner,omitempty"`
	AgentVersion string           `json:"agent-version,omitempty" yaml:"agent-version,omitempty"`
}

func (c *modelsCommand) modelSummaryFromParams(apiSummary base.UserModelSummary, now time.Time) (ModelSummary, error) {
	summary := ModelSummary{
		ShortName:      apiSummary.Name,
		Name:           jujuclient.JoinOwnerModelName(names.NewUserTag(apiSummary.Owner), apiSummary.Name),
		UUID:           apiSummary.UUID,
		Type:           apiSummary.Type,
		ControllerUUID: apiSummary.ControllerUUID,
		Owner:          apiSummary.Owner,
		Life:           apiSummary.Life,
		Cloud:          apiSummary.Cloud,
		CloudRegion:    apiSummary.CloudRegion,
		UserAccess:     apiSummary.ModelUserAccess,
		Status: &common.ModelStatus{
			Current: apiSummary.Status.Status,
			Message: apiSummary.Status.Info,
			Since:   common.FriendlyDuration(apiSummary.Status.Since, now),
		},
	}
	if apiSummary.AgentVersion != nil {
		summary.AgentVersion = apiSummary.AgentVersion.String()
	}
	if apiSummary.Migration != nil {
		status := summary.Status
		if status == nil {
			status = &common.ModelStatus{}
			summary.Status = status
		}
		status.Migration = apiSummary.Migration.Status
		status.MigrationStart = common.FriendlyDuration(apiSummary.Migration.StartTime, now)
		status.MigrationEnd = common.FriendlyDuration(apiSummary.Migration.EndTime, now)
	}

	if apiSummary.ProviderType != "" {
		summary.ProviderType = apiSummary.ProviderType

	}
	if apiSummary.CloudCredential != "" {
		credTag := names.NewCloudCredentialTag(apiSummary.CloudCredential)
		summary.CloudCredential = &common.ModelCredential{
			Name:  credTag.Name(),
			Owner: credTag.Owner().Id(),
			Cloud: credTag.Cloud().Id(),
		}
	}
	if apiSummary.UserLastConnection != nil {
		summary.UserLastConnection = common.UserFriendlyDuration(*apiSummary.UserLastConnection, now)
	} else {
		summary.UserLastConnection = "never connected"
	}
	if apiSummary.SLA != nil {
		summary.SLA = apiSummary.SLA.Level
		summary.SLAOwner = apiSummary.SLA.Owner
	}
	summary.Counts = map[string]int64{}
	for _, v := range apiSummary.Counts {
		summary.Counts[string(v.Entity)] = v.Count
	}

	// If hasMachinesCounts is not yet set, check if we should set it based on this model summary.
	if !c.runVars.hasMachinesCount {
		if _, ok := summary.Counts[string(params.Machines)]; ok {
			c.runVars.hasMachinesCount = true
		}
	}

	// If hasCoresCounts is not yet set, check if we should set it based on this model summary.
	if !c.runVars.hasCoresCount {
		if _, ok := summary.Counts[string(params.Cores)]; ok {
			c.runVars.hasCoresCount = true
		}
	}
	return summary, nil
}

// These values are specific to an individual Run() of the model command.
type modelsRunValues struct {
	currentUser      names.UserTag
	controllerName   string
	hasMachinesCount bool
	hasCoresCount    bool
}

// formatTabular takes an interface{} to adhere to the cmd.Formatter interface
func (c *modelsCommand) formatTabular(writer io.Writer, value interface{}) error {
	summariesSet, ok := value.(ModelSummarySet)
	if !ok {
		modelSet, k := value.(ModelSet)
		if !k {
			return errors.Errorf("expected value of type ModelSummarySet or ModelSet, got %T", value)
		}
		return c.tabularSet(writer, modelSet)
	}
	return c.tabularSummaries(writer, summariesSet)
}

func (c *modelsCommand) tabularColumns(tw *ansiterm.TabWriter, w output.Wrapper) {
	w.Println("Controller: " + c.runVars.controllerName)
	w.Println()
	w.Print("Model")
	if c.listUUID {
		w.Print("UUID")
	}
	w.Print("Cloud/Region", "Status")
	printColumnHeader := func(columnName string, columnNumber int) {
		w.Print(columnName)
		offset := 0
		if c.listUUID {
			offset++
		}
		tw.SetColumnAlignRight(columnNumber + offset)
	}

	if c.runVars.hasMachinesCount {
		printColumnHeader("Machines", 3)
	}

	if c.runVars.hasCoresCount {
		printColumnHeader("Cores", 4)
	}
	w.Println("Access", "Last connection")
}

// tabularSummaries takes model summaries set to adhere to the cmd.Formatter interface
func (c *modelsCommand) tabularSummaries(writer io.Writer, modelSet ModelSummarySet) error {
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}
	c.tabularColumns(tw, w)

	for _, model := range modelSet.Models {
		cloudRegion := strings.Trim(model.Cloud+"/"+model.CloudRegion, "/")
		owner := names.NewUserTag(model.Owner)
		name := model.Name
		if c.runVars.currentUser == owner {
			// No need to display fully qualified model name to its owner.
			name = model.ShortName
		}
		if model.Name == modelSet.CurrentModelQualified {
			name += "*"
			w.PrintColor(output.CurrentHighlight, name)
		} else {
			w.Print(name)
		}
		if c.listUUID {
			w.Print(model.UUID)
		}
		status := "-"
		if model.Status != nil && model.Status.Current.String() != "" {
			status = model.Status.Current.String()
		}
		w.Print(cloudRegion, status)
		if c.runVars.hasMachinesCount {
			if v, ok := model.Counts[string(params.Machines)]; ok {
				w.Print(v)
			} else {
				w.Print(0)
			}
		}
		if c.runVars.hasCoresCount {
			if v, ok := model.Counts[string(params.Cores)]; ok {
				w.Print(v)
			} else {
				w.Print("-")
			}
		}
		access := model.UserAccess
		if access == "" {
			access = "-"
		}
		w.Println(access, model.UserLastConnection)
	}
	tw.Flush()
	return nil
}

var listModelsDoc = `
The models listed here are either models you have created yourself, or
models which have been shared with you. Default values for user and
controller are, respectively, the current user and the current controller.
The active model is denoted by an asterisk.

Examples:

    juju models
    juju models --user bob

See also:
    add-model
    share-model
    unshare-model
`
