// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclienttesting

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/jujuclient"
)

// MemStore is an in-memory implementation of jujuclient.ClientStore,
// intended for testing.
type MemStore struct {
	Controllers           map[string]jujuclient.ControllerDetails
	CurrentControllerName string
	Models                map[string]*jujuclient.ControllerModels
	Accounts              map[string]jujuclient.AccountDetails
	Credentials           map[string]cloud.CloudCredential
	BootstrapConfig       map[string]jujuclient.BootstrapConfig
}

func NewMemStore() *MemStore {
	return &MemStore{
		Controllers:     make(map[string]jujuclient.ControllerDetails),
		Models:          make(map[string]*jujuclient.ControllerModels),
		Accounts:        make(map[string]jujuclient.AccountDetails),
		Credentials:     make(map[string]cloud.CloudCredential),
		BootstrapConfig: make(map[string]jujuclient.BootstrapConfig),
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

// CurrentController implements ControllerGetter.CurrentController
func (c *MemStore) CurrentController() (string, error) {
	if c.CurrentControllerName == "" {
		return "", errors.NotFoundf("current controller")
	}
	return c.CurrentControllerName, nil
}

// SetCurrentController implements ControllerUpdater.SetCurrentController
func (c *MemStore) SetCurrentController(name string) error {
	if err := jujuclient.ValidateControllerName(name); err != nil {
		return err
	}
	if _, ok := c.Controllers[name]; !ok {
		return errors.NotFoundf("controller %s", name)
	}
	c.CurrentControllerName = name
	return nil
}

// AddController implements ControllerUpdater.AddController
func (c *MemStore) AddController(name string, one jujuclient.ControllerDetails) error {
	if err := jujuclient.ValidateControllerName(name); err != nil {
		return err
	}
	if err := jujuclient.ValidateControllerDetails(one); err != nil {
		return err
	}

	if _, ok := c.Controllers[name]; ok {
		return errors.AlreadyExistsf("controller with name %s", name)
	}

	for k, v := range c.Controllers {
		if v.ControllerUUID == one.ControllerUUID {
			return errors.AlreadyExistsf("controller with UUID %s (%s)",
				one.ControllerUUID, k)
		}
	}
	c.Controllers[name] = one
	return nil
}

// UpdateController implements ControllerUpdater.UpdateController
func (c *MemStore) UpdateController(name string, one jujuclient.ControllerDetails) error {
	if err := jujuclient.ValidateControllerName(name); err != nil {
		return err
	}
	if err := jujuclient.ValidateControllerDetails(one); err != nil {
		return err
	}

	if len(c.Controllers) == 0 {
		return errors.NotFoundf("controllers")
	}

	for k, v := range c.Controllers {
		if v.ControllerUUID == one.ControllerUUID && k != name {
			return errors.AlreadyExistsf("controller %s with UUID %s",
				k, v.ControllerUUID)
		}
	}

	if _, ok := c.Controllers[name]; !ok {
		return errors.NotFoundf("controller %s", name)
	}

	c.Controllers[name] = one
	return nil
}

// RemoveController implements ControllerRemover.RemoveController
func (c *MemStore) RemoveController(name string) error {
	if err := jujuclient.ValidateControllerName(name); err != nil {
		return err
	}
	names := set.NewStrings(name)
	if namedControllerDetails, ok := c.Controllers[name]; ok {
		for name, details := range c.Controllers {
			if details.ControllerUUID == namedControllerDetails.ControllerUUID {
				names.Add(name)
				if name == c.CurrentControllerName {
					c.CurrentControllerName = ""
				}
			}
		}
	}
	for _, name := range names.Values() {
		delete(c.Models, name)
		delete(c.Accounts, name)
		delete(c.BootstrapConfig, name)
		delete(c.Controllers, name)
	}
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
	controllerModels, ok := c.Models[controller]
	if !ok {
		controllerModels = &jujuclient.ControllerModels{
			Models: make(map[string]jujuclient.ModelDetails),
		}
		c.Models[controller] = controllerModels
	}
	controllerModels.Models[model] = details
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
	controllerModels, ok := c.Models[controllerName]
	if !ok {
		return errors.NotFoundf("models for controller %s", controllerName)
	}
	if _, ok := controllerModels.Models[modelName]; !ok {
		return errors.NotFoundf("model %s:%s", controllerName, modelName)
	}
	controllerModels.CurrentModel = modelName
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
	controllerModels, ok := c.Models[controller]
	if !ok {
		return errors.NotFoundf("models for controller %s", controller)
	}
	if _, ok := controllerModels.Models[model]; !ok {
		return errors.NotFoundf("model %s:%s", controller, model)
	}
	delete(controllerModels.Models, model)
	if controllerModels.CurrentModel == model {
		controllerModels.CurrentModel = ""
	}
	return nil
}

// AllModels implements ModelGetter.
func (c *MemStore) AllModels(controller string) (map[string]jujuclient.ModelDetails, error) {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return nil, err
	}
	controllerModels, ok := c.Models[controller]
	if !ok {
		return nil, errors.NotFoundf("models for controller %s", controller)
	}
	return controllerModels.Models, nil
}

// CurrentModel implements ModelGetter.
func (c *MemStore) CurrentModel(controller string) (string, error) {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return "", err
	}
	controllerModels, ok := c.Models[controller]
	if !ok {
		return "", errors.NotFoundf("models for controller %s", controller)
	}
	if controllerModels.CurrentModel == "" {
		return "", errors.NotFoundf("current model for controller %s", controller)
	}
	return controllerModels.CurrentModel, nil
}

// ModelByName implements ModelGetter.
func (c *MemStore) ModelByName(controller, model string) (*jujuclient.ModelDetails, error) {
	if err := jujuclient.ValidateControllerName(controller); err != nil {
		return nil, err
	}
	if err := jujuclient.ValidateModelName(model); err != nil {
		return nil, err
	}
	controllerModels, ok := c.Models[controller]
	if !ok {
		return nil, errors.NotFoundf("models for controller %s", controller)
	}
	details, ok := controllerModels.Models[model]
	if !ok {
		return nil, errors.NotFoundf("model %s:%s", controller, model)
	}
	return &details, nil
}

// UpdateAccount implements AccountUpdater.
func (c *MemStore) UpdateAccount(controllerName string, details jujuclient.AccountDetails) error {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return err
	}
	if err := jujuclient.ValidateAccountDetails(details); err != nil {
		return err
	}
	oldDetails := c.Accounts[controllerName]
	// Only update last known access if it has a value.
	if details.LastKnownAccess == "" {
		details.LastKnownAccess = oldDetails.LastKnownAccess
	}
	c.Accounts[controllerName] = details
	return nil
}

// AccountDetails implements AccountGetter.
func (c *MemStore) AccountDetails(controllerName string) (*jujuclient.AccountDetails, error) {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return nil, err
	}
	details, ok := c.Accounts[controllerName]
	if !ok {
		return nil, errors.NotFoundf("account for controller %s", controllerName)
	}
	return &details, nil
}

// RemoveAccount implements AccountRemover.
func (c *MemStore) RemoveAccount(controllerName string) error {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return err
	}
	if _, ok := c.Accounts[controllerName]; !ok {
		return errors.NotFoundf("account for controller %s", controllerName)
	}
	delete(c.Accounts, controllerName)
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

// UpdateBootstrapConfig implements BootstrapConfigUpdater.
func (c *MemStore) UpdateBootstrapConfig(controllerName string, cfg jujuclient.BootstrapConfig) error {
	if err := jujuclient.ValidateControllerName(controllerName); err != nil {
		return err
	}
	if err := jujuclient.ValidateBootstrapConfig(cfg); err != nil {
		return err
	}
	c.BootstrapConfig[controllerName] = cfg
	return nil

}

// BootstrapConfigForController implements BootstrapConfigGetter.
func (c *MemStore) BootstrapConfigForController(controllerName string) (*jujuclient.BootstrapConfig, error) {
	if cfg, ok := c.BootstrapConfig[controllerName]; ok {
		return &cfg, nil
	}
	return nil, errors.NotFoundf("bootstrap config for controller %s", controllerName)

}
