// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/jujuclient"
)

// ControllerSet contains the set of controllers known to the client,
// and name of the current controller.
type ControllerSet struct {
	Controllers       map[string]ControllerItem `yaml:"controllers" json:"controllers"`
	CurrentController string                    `yaml:"current-controller" json:"current-controller"`
}

// ControllerItem defines the serialization behaviour of controller information.
type ControllerItem struct {
	ModelName      string   `yaml:"current-model,omitempty" json:"current-model,omitempty"`
	User           string   `yaml:"user,omitempty" json:"user,omitempty"`
	Server         string   `yaml:"recent-server,omitempty" json:"recent-server,omitempty"`
	Servers        []string `yaml:"servers,flow" json:"servers"`
	ControllerUUID string   `yaml:"uuid" json:"uuid"`
	APIEndpoints   []string `yaml:"api-endpoints,flow" json:"api-endpoints"`
	CACert         string   `yaml:"ca-cert" json:"ca-cert"`
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
		// The most recently connected-to host name
		// is the first in the list.
		if len(details.Servers) > 0 {
			serverName = details.Servers[0]
		}

		var userName, modelName string
		accountName, err := c.store.CurrentAccount(controllerName)
		if err != nil {
			if !errors.IsNotFound(err) {
				addError("account name", controllerName, err)
				continue
			}
		} else {
			currentAccount, err := c.store.AccountByName(controllerName, accountName)
			if err != nil {
				addError("account details", controllerName, err)
				continue
			}
			userName = currentAccount.User

			currentModel, err := c.store.CurrentModel(controllerName, accountName)
			if err != nil {
				if !errors.IsNotFound(err) {
					addError("model", controllerName, err)
					continue
				}
			}
			modelName = currentModel
		}

		controllers[controllerName] = ControllerItem{
			ModelName:      modelName,
			User:           userName,
			Server:         serverName,
			Servers:        details.Servers,
			APIEndpoints:   details.APIEndpoints,
			ControllerUUID: details.ControllerUUID,
			CACert:         details.CACert,
		}
	}
	return controllers, errs
}
