// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"bytes"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewListModelsCommand returns a command to list models.
func NewListModelsCommand() cmd.Command {
	return modelcmd.WrapController(&modelsCommand{})
}

// modelsCommand returns the list of all the models the
// current user can access on the current controller.
type modelsCommand struct {
	modelcmd.ControllerCommandBase
	out       cmd.Output
	all       bool
	user      string
	listUUID  bool
	exactTime bool
	modelAPI  ModelManagerAPI
	sysAPI    ModelsSysAPI
}

var listModelsDoc = `
The models listed here are either models you have created yourself, or
models which have been shared with you. Default values for user and
controller are, respectively, the current user and the current controller.
The active model is denoted by an asterisk.

Examples:

    juju list-models
    juju list-models --user bob

See also: add-model
          share-model
          unshare-model
`

// ModelManagerAPI defines the methods on the model manager API that
// the models command calls.
type ModelManagerAPI interface {
	Close() error
	ListModels(user string) ([]base.UserModel, error)
	ModelInfo([]names.ModelTag) ([]params.ModelInfoResult, error)
}

// ModelsSysAPI defines the methods on the controller manager API that the
// list models command calls.
type ModelsSysAPI interface {
	Close() error
	AllModels() ([]base.UserModel, error)
}

// Info implements Command.Info
func (c *modelsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-models",
		Purpose: "Lists models a user can access on a controller.",
		Doc:     listModelsDoc,
	}
}

func (c *modelsCommand) getModelManagerAPI() (ModelManagerAPI, error) {
	if c.modelAPI != nil {
		return c.modelAPI, nil
	}
	return c.NewModelManagerAPIClient()
}

func (c *modelsCommand) getSysAPI() (ModelsSysAPI, error) {
	if c.sysAPI != nil {
		return c.sysAPI, nil
	}
	return c.NewControllerAPIClient()
}

// SetFlags implements Command.SetFlags.
func (c *modelsCommand) SetFlags(f *gnuflag.FlagSet) {
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

// ModelSet contains the set of models known to the client,
// and UUID of the current model.
type ModelSet struct {
	Models       []common.ModelInfo `yaml:"models" json:"models"`
	CurrentModel string             `yaml:"current-model,omitempty" json:"current-model,omitempty"`
}

// Run implements Command.Run
func (c *modelsCommand) Run(ctx *cmd.Context) error {
	if c.user == "" {
		accountDetails, err := c.ClientStore().AccountByName(
			c.ControllerName(), c.AccountName(),
		)
		if err != nil {
			return err
		}
		c.user = accountDetails.User
	}

	// First get a list of the models.
	var models []base.UserModel
	var err error
	if c.all {
		models, err = c.getAllModels()
	} else {
		models, err = c.getUserModels()
	}
	if err != nil {
		return errors.Annotate(err, "cannot list models")
	}

	// And now get the full details of the models.
	paramsModelInfo, err := c.getModelInfo(models)
	if err != nil {
		return errors.Annotate(err, "cannot get model details")
	}

	// TODO(perrito666) 2016-05-02 lp:1558657
	now := time.Now()
	modelInfo := make([]common.ModelInfo, 0, len(models))
	for _, info := range paramsModelInfo {
		model, err := common.ModelInfoFromParams(info, now)
		if err != nil {
			return errors.Trace(err)
		}
		modelInfo = append(modelInfo, model)
	}

	modelSet := ModelSet{Models: modelInfo}
	current, err := c.ClientStore().CurrentModel(c.ControllerName(), c.AccountName())
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	modelSet.CurrentModel = current
	if err := c.out.Write(ctx, modelSet); err != nil {
		return err
	}

	if len(models) == 0 && c.out.Name() == "tabular" {
		// When the output is tabular, we inform the user when there
		// are no models available, and tell them how to go about
		// creating or granting access to them.
		fmt.Fprintf(ctx.Stderr, "\n%s\n\n", errNoModels.Error())
	}
	return nil
}

func (c *modelsCommand) getModelInfo(userModels []base.UserModel) ([]params.ModelInfo, error) {
	client, err := c.getModelManagerAPI()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()

	tags := make([]names.ModelTag, len(userModels))
	for i, m := range userModels {
		tags[i] = names.NewModelTag(m.UUID)
	}
	results, err := client.ModelInfo(tags)
	if err != nil {
		return nil, errors.Trace(err)
	}

	info := make([]params.ModelInfo, len(tags))
	for i, result := range results {
		if result.Error != nil {
			if params.IsCodeUnauthorized(result.Error) {
				// If we get this, then the model was removed
				// between the initial listing and the call
				// to query its details.
				continue
			}
			return nil, errors.Annotatef(
				result.Error, "getting model %s (%q) info",
				userModels[i].UUID, userModels[i].Name,
			)
		}
		info[i] = *result.Result
	}
	return info, nil
}

func (c *modelsCommand) getAllModels() ([]base.UserModel, error) {
	client, err := c.getSysAPI()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()
	return client.AllModels()
}

func (c *modelsCommand) getUserModels() ([]base.UserModel, error) {
	client, err := c.getModelManagerAPI()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()
	return client.ListModels(c.user)
}

// formatTabular takes an interface{} to adhere to the cmd.Formatter interface
func (c *modelsCommand) formatTabular(value interface{}) ([]byte, error) {
	modelSet, ok := value.(ModelSet)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", modelSet, value)
	}
	var out bytes.Buffer
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	fmt.Fprintf(tw, "MODEL")
	if c.listUUID {
		fmt.Fprintf(tw, "\tMODEL UUID")
	}
	fmt.Fprintf(tw, "\tOWNER\tSTATUS\tLAST CONNECTION\n")
	for _, model := range modelSet.Models {
		name := model.Name
		if name == modelSet.CurrentModel {
			name += "*"
		}
		fmt.Fprintf(tw, "%s", name)
		if c.listUUID {
			fmt.Fprintf(tw, "\t%s", model.UUID)
		}
		user := names.NewUserTag(c.user).Canonical()
		lastConnection := model.Users[user].LastConnection
		if lastConnection == "" {
			lastConnection = "never connected"
		}
		fmt.Fprintf(tw, "\t%s\t%s\t%s\n", model.Owner, model.Status.Current, lastConnection)
	}
	tw.Flush()
	return out.Bytes(), nil
}
