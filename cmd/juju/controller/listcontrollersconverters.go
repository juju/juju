// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/jujuclient"
)

// ControllerItem defines the serialization behaviour of controller information.
type ControllerItem struct {
	ModelName string `yaml:"model,omitempty" json:"model,omitempty"`
	User      string `yaml:"user,omitempty" json:"user,omitempty"`
	Server    string `yaml:"server,omitempty" json:"server,omitempty"`
}

// convertControllerDetails takes a map of Controllers and
// the recently used model for each and
// creates a list of amalgamated controller and model details.
// TODO (anastasiamac 2016-02-10) this would also take in details of latest used models soon.
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

		currentModel, err := c.newStoreFunc().CurrentModel(controllerName)
		if err != nil {
			if !errors.IsNotFound(err) {
				addError("model", controllerName, err)
				continue
			}
		}

		userName := ""
		accountName, err := c.newStoreFunc().CurrentAccount(controllerName)
		if err != nil {
			if !errors.IsNotFound(err) {
				addError("account name", controllerName, err)
				continue
			}
		} else {
			currentAccount, err := c.newStoreFunc().AccountByName(controllerName, accountName)
			if err != nil {
				addError("account details", controllerName, err)
				continue
			}
			userName = currentAccount.User
		}
		controllers[controllerName] = ControllerItem{
			currentModel,
			userName,
			serverName,
		}
	}
	return controllers, errs
}
