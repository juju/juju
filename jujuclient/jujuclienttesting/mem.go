// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclienttesting

import (
	"github.com/juju/errors"

	"github.com/juju/juju/jujuclient"
)

type inMemory struct {
	controllers map[string]jujuclient.ControllerDetails
	models      map[string]jujuclient.ControllerModels
}

func NewMemControllerStore() jujuclient.ControllerStore {
	return &inMemory{
		make(map[string]jujuclient.ControllerDetails),
		make(map[string]jujuclient.ControllerModels),
	}
}

// AllController implements ControllerGetter.AllController
func (c *inMemory) AllControllers() (map[string]jujuclient.ControllerDetails, error) {
	return c.controllers, nil
}

// ControllerByName implements ControllerGetter.ControllerByName
func (c *inMemory) ControllerByName(name string) (*jujuclient.ControllerDetails, error) {
	if err := jujuclient.ValidateControllerName(name); err != nil {
		return nil, err
	}
	if result, ok := c.controllers[name]; ok {
		return &result, nil
	}
	return nil, errors.NotFoundf("controller %s", name)
}

// UpdateController implements ControllerUpdater.UpdateController
func (c *inMemory) UpdateController(name string, one jujuclient.ControllerDetails) error {
	if err := jujuclient.ValidateControllerName(name); err != nil {
		return err
	}
	if err := jujuclient.ValidateControllerDetails(one); err != nil {
		return err
	}
	c.controllers[name] = one
	return nil
}

// RemoveController implements ControllerRemover.RemoveController
func (c *inMemory) RemoveController(name string) error {
	if err := jujuclient.ValidateControllerName(name); err != nil {
		return err
	}
	delete(c.controllers, name)
	return nil
}

// UpdateModel implements ModelUpdater.
func (c *inMemory) UpdateModel(controller, model string, details jujuclient.ModelDetails) error {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return err
	}
	if err := jujuclient.ValidateModelName(model); err != nil {
		return err
	}
	if err := jujuclient.ValidateModelDetails(details); err != nil {
		return err
	}
	models, ok := c.models[controller]
	if !ok {
		return errors.NotFoundf("controller %s", controller)
	}
	models.Models[model] = details
	return nil
}

// SetControllerModel implements ModelUpdater.
func (c *inMemory) SetControllerModel(controller, model string) error {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return err
	}
	if err := jujuclient.ValidateModelName(model); err != nil {
		return err
	}
	models, ok := c.models[controller]
	if !ok {
		return errors.NotFoundf("controller %s", controller)
	}
	if _, ok := models.Models[model]; !ok {
		return errors.NotFoundf("model %s:%s", controller, model)
	}
	models.CurrentModel = model
	return nil
}

// RemoveModel implements ModelRemover.
func (c *inMemory) RemoveModel(controller, model string) error {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return err
	}
	if err := jujuclient.ValidateModelName(model); err != nil {
		return err
	}
	models, ok := c.models[controller]
	if !ok {
		return errors.NotFoundf("controller %s", controller)
	}
	if _, ok := models.Models[model]; !ok {
		return errors.NotFoundf("model %s:%s", controller, model)
	}
	delete(models.Models, model)
	return nil
}

// AllModels implements ModelGetter.
func (c *inMemory) AllModels(controller string) (map[string]jujuclient.ModelDetails, error) {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return nil, err
	}
	models, ok := c.models[controller]
	if !ok {
		return nil, errors.NotFoundf("controller %s", controller)
	}
	return models.Models, nil
}

// CurrentModel implements ModelGetter.
func (c *inMemory) CurrentModel(controller string) (string, error) {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return "", err
	}
	models, ok := c.models[controller]
	if !ok {
		return "", errors.NotFoundf("controller %s", controller)
	}
	if models.CurrentModel == "" {
		return "", errors.NotFoundf("curernt model for controller %s", controller)
	}
	return models.CurrentModel, nil
}

// ModelByName implements ModelGetter.
func (c *inMemory) ModelByName(controller, model string) (*jujuclient.ModelDetails, error) {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return nil, err
	}
	if err := jujuclient.ValidateModelName(model); err != nil {
		return nil, err
	}
	models, ok := c.models[controller]
	if !ok {
		return nil, errors.NotFoundf("controller %s", controller)
	}
	details, ok := models.Models[model]
	if !ok {
		return nil, errors.NotFoundf("model %s:%s", controller, model)
	}
	return &details, nil
}
