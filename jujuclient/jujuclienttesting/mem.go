// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclienttesting

import (
	"github.com/juju/errors"

	"github.com/juju/juju/jujuclient"
)

// MemStore is an in-memory implementation of jujuclient.ClientStore,
// intended for testing.
type MemStore struct {
	Controllers map[string]jujuclient.ControllerDetails
	Models      map[string]*jujuclient.ControllerModels
	Accounts    map[string]*jujuclient.ControllerAccounts
}

func NewMemStore() *MemStore {
	return &MemStore{
		make(map[string]jujuclient.ControllerDetails),
		make(map[string]*jujuclient.ControllerModels),
		make(map[string]*jujuclient.ControllerAccounts),
	}
}

// AllController implements ControllerGetter.AllController
func (c *MemStore) AllControllers() (map[string]jujuclient.ControllerDetails, error) {
	return c.Controllers, nil
}

// ControllerByName implements ControllerGetter.ControllerByName
func (c *MemStore) ControllerByName(name string) (*jujuclient.ControllerDetails, error) {
	if err := jujuclient.ValidateControllerName(name); err != nil {
		return nil, err
	}
	if result, ok := c.Controllers[name]; ok {
		return &result, nil
	}
	return nil, errors.NotFoundf("controller %s", name)
}

// UpdateController implements ControllerUpdater.UpdateController
func (c *MemStore) UpdateController(name string, one jujuclient.ControllerDetails) error {
	if err := jujuclient.ValidateControllerName(name); err != nil {
		return err
	}
	if err := jujuclient.ValidateControllerDetails(one); err != nil {
		return err
	}
	c.Controllers[name] = one
	return nil
}

// RemoveController implements ControllerRemover.RemoveController
func (c *MemStore) RemoveController(name string) error {
	if err := jujuclient.ValidateControllerName(name); err != nil {
		return err
	}
	delete(c.Controllers, name)
	return nil
}

// UpdateModel implements ModelUpdater.
func (c *MemStore) UpdateModel(controller, model string, details jujuclient.ModelDetails) error {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return err
	}
	if err := jujuclient.ValidateModelName(model); err != nil {
		return err
	}
	if err := jujuclient.ValidateModelDetails(details); err != nil {
		return err
	}
	models, ok := c.Models[controller]
	if !ok {
		models = &jujuclient.ControllerModels{
			Models: make(map[string]jujuclient.ModelDetails),
		}
		c.Models[controller] = models
	}
	models.Models[model] = details
	return nil
}

// SetCurrentModel implements ModelUpdater.
func (c *MemStore) SetCurrentModel(controllerName, modelName string) error {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := jujuclient.ValidateModelName(modelName); err != nil {
		return errors.Trace(err)
	}
	models, ok := c.Models[controllerName]
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
func (c *MemStore) RemoveModel(controller, model string) error {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return err
	}
	if err := jujuclient.ValidateModelName(model); err != nil {
		return err
	}
	models, ok := c.Models[controller]
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
func (c *MemStore) AllModels(controller string) (map[string]jujuclient.ModelDetails, error) {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return nil, err
	}
	models, ok := c.Models[controller]
	if !ok {
		return nil, errors.NotFoundf("controller %s", controller)
	}
	return models.Models, nil
}

// CurrentModel implements ModelGetter.
func (c *MemStore) CurrentModel(controller string) (string, error) {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return "", err
	}
	models, ok := c.Models[controller]
	if !ok {
		return "", errors.NotFoundf("controller %s", controller)
	}
	if models.CurrentModel == "" {
		return "", errors.NotFoundf("current model for controller %s", controller)
	}
	return models.CurrentModel, nil
}

// ModelByName implements ModelGetter.
func (c *MemStore) ModelByName(controller, model string) (*jujuclient.ModelDetails, error) {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return nil, err
	}
	if err := jujuclient.ValidateModelName(model); err != nil {
		return nil, err
	}
	models, ok := c.Models[controller]
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
func (c *MemStore) UpdateAccount(controllerName, accountName string, details jujuclient.AccountDetails) error {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return err
	}
	if err := jujuclient.ValidateAccountName(accountName); err != nil {
		return err
	}
	if err := jujuclient.ValidateAccountDetails(details); err != nil {
		return err
	}
	accounts, ok := c.Accounts[controllerName]
	if !ok {
		accounts = &jujuclient.ControllerAccounts{
			Accounts: make(map[string]jujuclient.AccountDetails),
		}
		c.Accounts[controllerName] = accounts
	}
	accounts.Accounts[accountName] = details
	return nil

}

// SetCurrentAccount implements AccountUpdater.
func (c *MemStore) SetCurrentAccount(controllerName, accountName string) error {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return err
	}
	if err := jujuclient.ValidateAccountName(accountName); err != nil {
		return err
	}
	accounts, ok := c.Accounts[controllerName]
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
func (c *MemStore) AllAccounts(controllerName string) (map[string]jujuclient.AccountDetails, error) {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return nil, err
	}
	accounts, ok := c.Accounts[controllerName]
	if !ok {
		return nil, errors.NotFoundf("controller %s", controllerName)
	}
	return accounts.Accounts, nil
}

// CurrentAccount implements AccountGetter.
func (c *MemStore) CurrentAccount(controllerName string) (string, error) {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return "", err
	}
	accounts, ok := c.Accounts[controllerName]
	if !ok {
		return "", errors.NotFoundf("controller %s", controllerName)
	}
	if accounts.CurrentAccount == "" {
		return "", errors.NotFoundf("current account for controller %s", controllerName)
	}
	return accounts.CurrentAccount, nil
}

// AccountByName implements AccountGetter.
func (c *MemStore) AccountByName(controllerName, accountName string) (*jujuclient.AccountDetails, error) {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return nil, err
	}
	if err := jujuclient.ValidateAccountName(accountName); err != nil {
		return nil, err
	}
	accounts, ok := c.Accounts[controllerName]
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
func (c *MemStore) RemoveAccount(controllerName, accountName string) error {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return err
	}
	if err := jujuclient.ValidateAccountName(accountName); err != nil {
		return err
	}
	accounts, ok := c.Accounts[controllerName]
	if !ok {
		return errors.NotFoundf("controller %s", controllerName)
	}
	if _, ok := accounts.Accounts[accountName]; !ok {
		return errors.NotFoundf("account %s:%s", controllerName, accountName)
	}
	delete(accounts.Accounts, accountName)
	return nil
}
