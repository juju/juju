// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"sort"

	"github.com/juju/juju/jujuclient"
)

// ControllerItem defines the serialization behaviour of controller information.
type ControllerItem struct {
	ControllerName string `yaml:"controller" json:"controller"`
	ModelName      string `yaml:"model,omitempty" json:"model,omitempty"`
	User           string `yaml:"user,omitempty" json:"user,omitempty"`
	Server         string `yaml:"server,omitempty" json:"server,omitempty"`
}

// convertControllerDetails takes a map of Controllers and
// the recently used model for each and
// creates a list of amalgamated controller and model details.
// TODO (anastasiamac 2016-02-10) this would also take in details of latest used models soon.
func convertControllerDetails(storeControllers map[string]jujuclient.ControllerDetails) controllerList {
	if len(storeControllers) == 0 {
		return nil
	}
	list := []ControllerItem{}
	for name, _ := range storeControllers {
		item := ControllerItem{
			name,
			// TODO (anastaismac 2016-02-10) model name
			"",
			// TODO (anastaismac 2016-02-10) currently logged in user.
			// Remember, latest model does not mean that the user is still logged in to its controller.
			// Do I already know that?
			"",
			// TODO (anastaismac 2016-02-10) server
			"",
		}
		list = append(list, item)
	}
	sort.Sort(controllerList(list))
	return list
}

type controllerList []ControllerItem

func (l controllerList) Len() int {
	return len(l)
}

func (l controllerList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

func (l controllerList) Less(a, b int) bool {
	return l[a].ControllerName < l[b].ControllerName
}
