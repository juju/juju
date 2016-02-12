// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclienttesting

import (
	"github.com/juju/errors"

	"github.com/juju/juju/jujuclient"
)

type StubStore struct {
	Msg string
}

// AllControllers implements ControllersGetter.AllControllers
func (c *StubStore) AllControllers() (map[string]jujuclient.ControllerDetails, error) {
	return nil, errors.New(c.Msg)
}

// ControllerByName implements ControllersGetter.ControllerByName
func (c *StubStore) ControllerByName(name string) (*jujuclient.ControllerDetails, error) {
	panic("not for test")
}

// UpdateController implements ControllersUpdater.UpdateController
func (c *StubStore) UpdateController(name string, one jujuclient.ControllerDetails) error {
	panic("not for test")
}

// RemoveController implements ControllersRemover.RemoveController
func (c *StubStore) RemoveController(name string) error {
	panic("not for test")
}

// UpdateModel implements ModelUpdater.
func (c *StubStore) UpdateModel(controller, model string, details jujuclient.ModelDetails) error {
	return nil
}

// SetControllerModel implements ModelUpdater.
func (c *StubStore) SetControllerModel(controller, model string) error {
	return nil
}

// RemoveModel implements ModelRemover.
func (c *StubStore) RemoveModel(controller, model string) error {
	return nil
}

// AllModels implements ModelGetter.
func (c *StubStore) AllModels(controller string) (map[string]jujuclient.ModelDetails, error) {
	return nil, nil
}

// CurrentModel implements ModelGetter.
func (c *StubStore) CurrentModel(controller string) (string, error) {
	return "", nil
}

// ModelByName implements ModelGetter.
func (c *StubStore) ModelByName(controller, model string) (*jujuclient.ModelDetails, error) {
	return nil, nil
}
