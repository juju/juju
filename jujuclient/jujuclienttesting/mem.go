// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclienttesting

import (
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/jujuclient"
)

// MemStore is an in-memory implementation of jujuclient.ClientStore,
// intended for testing.
type MemStore struct {
	Controllers map[string]jujuclient.ControllerDetails
	Models      map[string]jujuclient.ControllerAccountModels
	Accounts    map[string]*jujuclient.ControllerAccounts
	Credentials map[string]cloud.CloudCredential
}

func NewMemStore() *MemStore {
	return &MemStore{
		make(map[string]jujuclient.ControllerDetails),
		make(map[string]jujuclient.ControllerAccountModels),
		make(map[string]*jujuclient.ControllerAccounts),
		make(map[string]cloud.CloudCredential),
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
func (c *MemStore) UpdateModel(controller, account, model string, details jujuclient.ModelDetails) error {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return err
	}
	if err := jujuclient.ValidateAccountName(account); err != nil {
		return err
	}
	if err := jujuclient.ValidateModelName(model); err != nil {
		return err
	}
	if err := jujuclient.ValidateModelDetails(details); err != nil {
		return err
	}
	controllerAccountModels, ok := c.Models[controller]
	if !ok {
		controllerAccountModels.AccountModels = make(map[string]*jujuclient.AccountModels)
		c.Models[controller] = controllerAccountModels
	}
	accountModels, ok := controllerAccountModels.AccountModels[account]
	if !ok {
		accountModels = &jujuclient.AccountModels{
			Models: make(map[string]jujuclient.ModelDetails),
		}
		controllerAccountModels.AccountModels[account] = accountModels
	}
	accountModels.Models[model] = details
	return nil
}

// SetCurrentModel implements ModelUpdater.
func (c *MemStore) SetCurrentModel(controllerName, accountName, modelName string) error {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := jujuclient.ValidateAccountName(accountName); err != nil {
		return err
	}
	if err := jujuclient.ValidateModelName(modelName); err != nil {
		return errors.Trace(err)
	}
	controllerAccountModels, ok := c.Models[controllerName]
	if !ok {
		return errors.NotFoundf("models for controller %s", controllerName)
	}
	accountModels, ok := controllerAccountModels.AccountModels[accountName]
	if !ok {
		return errors.NotFoundf(
			"models for account %s on controller %s",
			accountName, controllerName,
		)
	}
	if _, ok := accountModels.Models[modelName]; !ok {
		return errors.NotFoundf("model %s:%s:%s", controllerName, accountName, modelName)
	}
	accountModels.CurrentModel = modelName
	return nil
}

// RemoveModel implements ModelRemover.
func (c *MemStore) RemoveModel(controller, account, model string) error {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return err
	}
	if err := jujuclient.ValidateAccountName(account); err != nil {
		return err
	}
	if err := jujuclient.ValidateModelName(model); err != nil {
		return err
	}
	controllerAccountModels, ok := c.Models[controller]
	if !ok {
		return errors.NotFoundf("models for controller %s", controller)
	}
	accountModels, ok := controllerAccountModels.AccountModels[account]
	if !ok {
		return errors.NotFoundf(
			"models for account %s on controller %s",
			account, controller,
		)
	}
	if _, ok := accountModels.Models[model]; !ok {
		return errors.NotFoundf("model %s:%s:%s", controller, account, model)
	}
	delete(accountModels.Models, model)
	if accountModels.CurrentModel == model {
		accountModels.CurrentModel = ""
	}
	return nil
}

// AllModels implements ModelGetter.
func (c *MemStore) AllModels(controller, account string) (map[string]jujuclient.ModelDetails, error) {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return nil, err
	}
	if err := jujuclient.ValidateAccountName(account); err != nil {
		return nil, err
	}
	controllerAccountModels, ok := c.Models[controller]
	if !ok {
		return nil, errors.NotFoundf("models for controller %s", controller)
	}
	accountModels, ok := controllerAccountModels.AccountModels[account]
	if !ok {
		return nil, errors.NotFoundf("models for account %s on controller %s", account, controller)
	}
	return accountModels.Models, nil
}

// CurrentModel implements ModelGetter.
func (c *MemStore) CurrentModel(controller, account string) (string, error) {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return "", err
	}
	if err := jujuclient.ValidateAccountName(account); err != nil {
		return "", err
	}
	controllerAccountModels, ok := c.Models[controller]
	if !ok {
		return "", errors.NotFoundf("models for controller %s", controller)
	}
	accountModels, ok := controllerAccountModels.AccountModels[account]
	if !ok {
		return "", errors.NotFoundf("models for account %s on controller %s", account, controller)
	}
	if accountModels.CurrentModel == "" {
		return "", errors.NotFoundf("current model for account %s on controller %s", account, controller)
	}
	return accountModels.CurrentModel, nil
}

// ModelByName implements ModelGetter.
func (c *MemStore) ModelByName(controller, account, model string) (*jujuclient.ModelDetails, error) {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return nil, err
	}
	if err := jujuclient.ValidateAccountName(account); err != nil {
		return nil, err
	}
	if err := jujuclient.ValidateModelName(model); err != nil {
		return nil, err
	}
	controllerAccountModels, ok := c.Models[controller]
	if !ok {
		return nil, errors.NotFoundf("models for controller %s", controller)
	}
	accountModels, ok := controllerAccountModels.AccountModels[account]
	if !ok {
		return nil, errors.NotFoundf("models for account %s on controller %s", account, controller)
	}
	details, ok := accountModels.Models[model]
	if !ok {
		return nil, errors.NotFoundf("model %s:%s:%s", controller, account, model)
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
		return errors.NotFoundf("accounts for controller %s", controllerName)
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
		return nil, errors.NotFoundf("accounts for controller %s", controllerName)
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
		return "", errors.NotFoundf("accounts for controller %s", controllerName)
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
		return nil, errors.NotFoundf("accounts for controller %s", controllerName)
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
		return errors.NotFoundf("accounts for controller %s", controllerName)
	}
	if _, ok := accounts.Accounts[accountName]; !ok {
		return errors.NotFoundf("account %s:%s", controllerName, accountName)
	}
	delete(accounts.Accounts, accountName)
	return nil
}

// UpdateCredential implements CredentialsUpdater.
func (c *MemStore) UpdateCredential(cloudName string, details cloud.CloudCredential) error {
	c.Credentials[cloudName] = details
	return nil
}

// CredentialForCloud implements CredentialsGetter.
func (c *MemStore) CredentialForCloud(cloudName string) (*cloud.CloudCredential, error) {
	if result, ok := c.Credentials[cloudName]; ok {
		return &result, nil
	}
	return nil, errors.NotFoundf("credentials for cloud %s", cloudName)
}

// AllCredentials implements CredentialsGetter.
func (c *MemStore) AllCredentials() (map[string]cloud.CloudCredential, error) {
	result := make(map[string]cloud.CloudCredential)
	for k, v := range c.Credentials {
		result[k] = v
	}
	return result, nil
}
