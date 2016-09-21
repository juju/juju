// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/jujuclient"
)

// ControllerSet contains the set of controllers known to the client,
// and name of the current controller.
type ControllerSet struct {
	Controllers       map[string]ControllerItem `yaml:"controllers" json:"controllers"`
	CurrentController string                    `yaml:"current-controller" json:"current-controller"`
}

// ControllerMachines holds the total number of controller
// machines and the number of active ones.
type ControllerMachines struct {
	Active int `yaml:"active"`
	Total  int `yaml:"total"`
}

// ControllerItem defines the serialization behaviour of controller information.
type ControllerItem struct {
	ModelName          string              `yaml:"current-model,omitempty" json:"current-model,omitempty"`
	User               string              `yaml:"user,omitempty" json:"user,omitempty"`
	Access             string              `yaml:"access,omitempty" json:"access,omitempty"`
	Server             string              `yaml:"recent-server,omitempty" json:"recent-server,omitempty"`
	ControllerUUID     string              `yaml:"uuid" json:"uuid"`
	APIEndpoints       []string            `yaml:"api-endpoints,flow" json:"api-endpoints"`
	CACert             string              `yaml:"ca-cert" json:"ca-cert"`
	Cloud              string              `yaml:"cloud" json:"cloud"`
	CloudRegion        string              `yaml:"region,omitempty" json:"region,omitempty"`
	AgentVersion       string              `yaml:"agent-version,omitempty" json:"agent-version,omitempty"`
	ModelCount         *int                `yaml:"model-count,omitempty" json:"model-count,omitempty"`
	MachineCount       *int                `yaml:"machine-count,omitempty" json:"machine-count,omitempty"`
	ControllerMachines *ControllerMachines `yaml:"controller-machines,omitempty" json:"controller-machins,omitempty"`
}

// convertControllerDetails takes a map of Controllers and
// the recently used model for each and creates a list of
// amalgamated controller and model details.
func (c *listControllersCommand) convertControllerDetails(storeControllers map[string]jujuclient.ControllerDetails) (map[string]ControllerItem, []string) {
	if len(storeControllers) == 0 {
		return nil, nil
	}

	errs := []string{}
	addError := func(msg, controllerName string, err error) {
		logger.Errorf(fmt.Sprintf("getting current %s for controller %s: %v", msg, controllerName, err))
		errs = append(errs, msg)
	}

	controllers := map[string]ControllerItem{}
	for controllerName, details := range storeControllers {
		serverName := ""
		// The most recently connected-to address
		// is the first in the list.
		if len(details.APIEndpoints) > 0 {
			serverName = details.APIEndpoints[0]
		}

		var userName, access string
		accountDetails, err := c.store.AccountDetails(controllerName)
		if err != nil {
			if !errors.IsNotFound(err) {
				addError("account details", controllerName, err)
				continue
			}
		} else {
			userName = accountDetails.User
			access = accountDetails.LastKnownAccess
		}

		var modelName string
		currentModel, err := c.store.CurrentModel(controllerName)
		if err != nil {
			if !errors.IsNotFound(err) {
				addError("model", controllerName, err)
				continue
			}
		} else {
			modelName = currentModel
			if userName != "" {
				// There's a user logged in, so display the
				// model name relative to that user.
				if unqualifiedModelName, owner, err := jujuclient.SplitModelName(modelName); err == nil {
					user := names.NewUserTag(userName)
					modelName = common.OwnerQualifiedModelName(unqualifiedModelName, owner, user)
				}
			}
		}

		item := ControllerItem{
			ModelName:      modelName,
			User:           userName,
			Access:         access,
			Server:         serverName,
			APIEndpoints:   details.APIEndpoints,
			ControllerUUID: details.ControllerUUID,
			CACert:         details.CACert,
			Cloud:          details.Cloud,
			CloudRegion:    details.CloudRegion,
			AgentVersion:   details.AgentVersion,
		}
		if details.MachineCount != nil && *details.MachineCount > 0 {
			item.MachineCount = details.MachineCount
		}
		if details.ModelCount != nil && *details.ModelCount > 0 {
			item.ModelCount = details.ModelCount
		}
		if details.ControllerMachineCount > 0 {
			item.ControllerMachines = &ControllerMachines{
				Total:  details.ControllerMachineCount,
				Active: details.ActiveControllerMachineCount,
			}
		}
		controllers[controllerName] = item
	}
	return controllers, errs
}
