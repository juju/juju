// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/jujuclient"
)

// oldModelsCommandBehaviour is the 'models' behavior for pre Juju 2.3.
// This call:
// * gets all possible models (api calls)
// * translates them into user-friendly representation (presentation layer)
// * stores retrieved models in client store
// * identifies current model
// Call returns whether any models hav been found and any errors.
func (c *modelsCommand) oldModelsCommandBehaviour(
	ctx *cmd.Context,
	modelmanagerAPI ModelManagerAPI,
	now time.Time,
) (bool, error) {
	var models []base.UserModel
	var err error
	if c.all {
		models, err = c.getAllModels()
	} else {
		models, err = c.getUserModels(modelmanagerAPI)
	}
	if err != nil {
		return false, errors.Annotate(err, "cannot list models")
	}
	modelInfo, modelsToStore, err := c.getModelInfo(modelmanagerAPI, now, models)
	if err != nil {
		return false, errors.Annotate(err, "cannot get model details")
	}
	found := len(modelInfo) > 0

	if err := c.ClientStore().SetModels(c.runVars.controllerName, modelsToStore); err != nil {
		return found, errors.Trace(err)
	}

	// Identifying current model has to be done after models in client store have been updated
	// since that call determines/updates current model information.
	modelSet := ModelSet{Models: modelInfo}
	modelSet.CurrentModelQualified, modelSet.CurrentModel = c.currentModelName()
	if err := c.out.Write(ctx, modelSet); err != nil {
		return found, err
	}
	return found, err
}

// (anastasiamac 2017-23-11) This is old, pre juju 2.3 implementation.
func (c *modelsCommand) getModelInfo(
	client ModelManagerAPI,
	now time.Time,
	userModels []base.UserModel,
) ([]common.ModelInfo, map[string]jujuclient.ModelDetails, error) {
	tags := make([]names.ModelTag, len(userModels))
	for i, m := range userModels {
		tags[i] = names.NewModelTag(m.UUID)
	}
	results, err := client.ModelInfo(tags)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	info := []common.ModelInfo{}
	modelsToStore := map[string]jujuclient.ModelDetails{}
	for i, result := range results {
		if result.Error != nil {
			if params.IsCodeUnauthorized(result.Error) {
				// If we get this, then the model was removed
				// between the initial listing and the call
				// to query its details.
				continue
			}
			return nil, nil, errors.Annotatef(
				result.Error, "getting model %s (%q) info",
				userModels[i].UUID, userModels[i].Name,
			)
		}

		model, err := common.ModelInfoFromParams(*result.Result, now)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		model.ControllerName = c.runVars.controllerName
		info = append(info, model)
		modelsToStore[model.Name] = jujuclient.ModelDetails{ModelUUID: model.UUID, ModelType: model.Type}

		if len(model.Machines) != 0 {
			c.runVars.hasMachinesCount = true
			for _, m := range model.Machines {
				if m.Cores != 0 {
					c.runVars.hasCoresCount = true
					break
				}
			}
		}
	}
	return info, modelsToStore, nil
}

func (c *modelsCommand) getAllModels() ([]base.UserModel, error) {
	client, err := c.getSysAPI()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()
	return client.AllModels()
}

// ModelsSysAPI defines the methods on the controller manager API that the
// list models command calls.
type ModelsSysAPI interface {
	Close() error
	AllModels() ([]base.UserModel, error)
}

func (c *modelsCommand) getSysAPI() (ModelsSysAPI, error) {
	if c.sysAPI != nil {
		return c.sysAPI, nil
	}
	return c.NewControllerAPIClient()
}

func (c *modelsCommand) getUserModels(client ModelManagerAPI) ([]base.UserModel, error) {
	return client.ListModels(c.user)
}

// ModelSet contains the set of models known to the client,
// and UUID of the current model.
// (anastasiamac 2017-23-11) This is old, pre juju 2.3 implementation.
type ModelSet struct {
	Models []common.ModelInfo `yaml:"models" json:"models"`

	// CurrentModel is the name of the current model, qualified for the
	// user for which we're listing models. i.e. for the user admin,
	// and the model admin/foo, this field will contain "foo"; for
	// bob and the same model, the field will contain "admin/foo".
	CurrentModel string `yaml:"current-model,omitempty" json:"current-model,omitempty"`

	// CurrentModelQualified is the fully qualified name for the current
	// model, i.e. having the format $owner/$model.
	CurrentModelQualified string `yaml:"-" json:"-"`
}

// formatTabular takes a model set to adhere to the cmd.Formatter interface
// (anastasiamac 2017-23-11) This is old, pre juju 2.3 implementation.
func (c *modelsCommand) tabularSet(writer io.Writer, modelSet ModelSet) error {
	// We need the tag of the user for which we're listing models,
	// and for the logged-in user. We use these below when formatting
	// the model display names.
	loggedInUser := names.NewUserTag(c.loggedInUser)
	userForLastConn := loggedInUser
	var currentUser names.UserTag
	if c.user != "" {
		currentUser = names.NewUserTag(c.user)
		userForLastConn = currentUser
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}
	c.tabularColumns(tw, w)

	for _, model := range modelSet.Models {
		cloudRegion := strings.Trim(model.Cloud+"/"+model.CloudRegion, "/")
		owner := names.NewUserTag(model.Owner)
		name := model.Name
		if currentUser == owner {
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
		userForAccess := loggedInUser
		if c.user != "" {
			userForAccess = names.NewUserTag(c.user)
		}
		status := "-"
		if model.Status != nil {
			status = model.Status.Current.String()
		}
		w.Print(cloudRegion, model.ProviderType, status)
		if c.runVars.hasMachinesCount {
			w.Print(fmt.Sprintf("%d", len(model.Machines)))
		}
		if c.runVars.hasCoresCount {
			cores := uint64(0)
			for _, m := range model.Machines {
				cores += m.Cores
			}
			coresInfo := "-"
			if cores > 0 {
				coresInfo = fmt.Sprintf("%d", cores)
			}
			w.Print(coresInfo)
		}
		access := model.Users[userForAccess.Id()].Access
		if access == "" {
			access = "-"
		}
		lastConnection := model.Users[userForLastConn.Id()].LastConnection
		if lastConnection == "" {
			lastConnection = "never connected"
		}
		w.Println(access, lastConnection)
	}
	tw.Flush()
	return nil
}
