// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclienttesting

import (
	"github.com/juju/errors"

	"github.com/juju/juju/jujuclient"
)

type inMemory struct {
	all map[string]jujuclient.ControllerDetails
}

func NewMemControllerStore() jujuclient.ControllerStore {
	return &inMemory{make(map[string]jujuclient.ControllerDetails)}
}

// AllControllers implements ControllersGetter.AllControllers
func (c *inMemory) AllControllers() (map[string]jujuclient.ControllerDetails, error) {
	return c.all, nil
}

// ControllerByName implements ControllersGetter.ControllerByName
func (c *inMemory) ControllerByName(name string) (*jujuclient.ControllerDetails, error) {
	if result, ok := c.all[name]; ok {
		return &result, nil
	}
	return nil, errors.NotFoundf("controller %s", name)
}

// UpdateController implements ControllersUpdater.UpdateController
func (c *inMemory) UpdateController(name string, one jujuclient.ControllerDetails) error {
	if err := jujuclient.ValidateControllerDetails(name, one); err != nil {
		return err
	}
	c.all[name] = one
	return nil
}

// RemoveController implements ControllersRemover.RemoveController
func (c *inMemory) RemoveController(name string) error {
	delete(c.all, name)
	return nil
}
