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
	accounts    map[string]jujuclient.ControllerAccounts
}

func NewMemControllerStore() jujuclient.ClientStore {
	return &inMemory{
		make(map[string]jujuclient.ControllerDetails),
		make(map[string]jujuclient.ControllerModels),
		make(map[string]jujuclient.ControllerAccounts),
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
		models = jujuclient.ControllerModels{
			Models: make(map[string]jujuclient.ModelDetails),
		}
		c.models[controller] = models
	}
	models.Models[model] = details
	return nil
}

// SetCurrentModel implements ModelUpdater.
func (c *inMemory) SetCurrentModel(controllerName, modelName string) error {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := jujuclient.ValidateModelName(modelName); err != nil {
		return errors.Trace(err)
	}
	models, ok := c.models[controllerName]
	if !ok {
		return errors.NotFoundf("controller %s", controllerName)
	}
	if models.CurrentModel == modelName {
		return nil
	}
	if _, ok := models.Models[modelName]; !ok {
		return errors.NotFoundf("model %s:%s", controllerName, modelName)
	}

	models.CurrentModel = modelName
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

// UpdateAccount implements AccountUpdater.
func (c *inMemory) UpdateAccount(controllerName, accountName string, details jujuclient.AccountDetails) error {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return err
	}
	if err := jujuclient.ValidateAccountName(accountName); err != nil {
		return err
	}
	if err := jujuclient.ValidateAccountDetails(details); err != nil {
		return err
	}
	accounts, ok := c.accounts[controllerName]
	if !ok {
		accounts = jujuclient.ControllerAccounts{
			Accounts: make(map[string]jujuclient.AccountDetails),
		}
		c.accounts[controllerName] = accounts
	}
	accounts.Accounts[accountName] = details
	return nil

}

// SetCurrentAccount implements AccountUpdater.
func (c *inMemory) SetCurrentAccount(controllerName, accountName string) error {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return err
	}
	if err := jujuclient.ValidateAccountName(accountName); err != nil {
		return err
	}
	accounts, ok := c.accounts[controllerName]
	if !ok {
		return errors.NotFoundf("controller %s", controllerName)
	}
	if _, ok := accounts.Accounts[accountName]; !ok {
		return errors.NotFoundf("account %s:%s", controllerName, accountName)
	}
	accounts.CurrentAccount = accountName
	return nil
}

// AllAccounts implements AccountGetter.
func (c *inMemory) AllAccounts(controllerName string) (map[string]jujuclient.AccountDetails, error) {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return nil, err
	}
	accounts, ok := c.accounts[controllerName]
	if !ok {
		return nil, errors.NotFoundf("controller %s", controllerName)
	}
	return accounts.Accounts, nil
}

// CurrentAccount implements AccountGetter.
func (c *inMemory) CurrentAccount(controllerName string) (string, error) {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return "", err
	}
	accounts, ok := c.accounts[controllerName]
	if !ok {
		return "", errors.NotFoundf("controller %s", controllerName)
	}
	if accounts.CurrentAccount == "" {
		return "", errors.NotFoundf("curernt account for controller %s", controllerName)
	}
	return accounts.CurrentAccount, nil
}

// AccountByName implements AccountGetter.
func (c *inMemory) AccountByName(controllerName, accountName string) (*jujuclient.AccountDetails, error) {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return nil, err
	}
	if err := jujuclient.ValidateAccountName(accountName); err != nil {
		return nil, err
	}
	accounts, ok := c.accounts[controllerName]
	if !ok {
		return nil, errors.NotFoundf("controller %s", controllerName)
	}
	details, ok := accounts.Accounts[accountName]
	if !ok {
		return nil, errors.NotFoundf("account %s:%s", controllerName, accountName)
	}
	return &details, nil
}

// RemoveAccount implements AccountRemover.
func (c *inMemory) RemoveAccount(controllerName, accountName string) error {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return err
	}
	if err := jujuclient.ValidateAccountName(accountName); err != nil {
		return err
	}
	accounts, ok := c.accounts[controllerName]
	if !ok {
		return errors.NotFoundf("controller %s", controllerName)
	}
	if _, ok := accounts.Accounts[accountName]; !ok {
		return errors.NotFoundf("account %s:%s", controllerName, accountName)
	}
	delete(accounts.Accounts, accountName)
	return nil
}
