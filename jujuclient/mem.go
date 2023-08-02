// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	cookiejar "github.com/juju/persistent-cookiejar"

	"github.com/juju/juju/cloud"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/environs/config"
)

// MemStore is an in-memory implementation of ClientStore.
type MemStore struct {
	mu sync.Mutex

	Controllers           map[string]ControllerDetails
	CurrentControllerName string
	Models                map[string]*ControllerModels
	Accounts              map[string]AccountDetails
	Credentials           map[string]cloud.CloudCredential
	BootstrapConfig       map[string]BootstrapConfig
	CookieJars            map[string]*cookiejar.Jar
	ImmutableAccount      bool
}

// NewMemStore returns a new MemStore.
func NewMemStore() *MemStore {
	return &MemStore{
		Controllers:     make(map[string]ControllerDetails),
		Models:          make(map[string]*ControllerModels),
		Accounts:        make(map[string]AccountDetails),
		Credentials:     make(map[string]cloud.CloudCredential),
		BootstrapConfig: make(map[string]BootstrapConfig),
		CookieJars:      make(map[string]*cookiejar.Jar),
	}
}

// NewEmbeddedMemStore returns a new MemStore used with the embedded CLI.
// The account details are immutable once set.
func NewEmbeddedMemStore() *MemStore {
	s := NewMemStore()
	s.ImmutableAccount = true
	return s
}

// AllControllers implements ControllerGetter.AllController
func (c *MemStore) AllControllers() (map[string]ControllerDetails, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make(map[string]ControllerDetails)
	for name, details := range c.Controllers {
		result[name] = details
	}
	return result, nil
}

// ControllerByName implements ControllerGetter.ControllerByName
func (c *MemStore) ControllerByName(name string) (*ControllerDetails, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(name); err != nil {
		return nil, err
	}
	if result, ok := c.Controllers[name]; ok {
		return &result, nil
	}
	return nil, errors.NotFoundf("controller %s", name)
}

// ControllerByAPIEndpoints implements ControllersGetter.ControllerByAPIEndpoints
func (c *MemStore) ControllerByAPIEndpoints(endpoints ...string) (*ControllerDetails, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	matchEps := set.NewStrings(endpoints...)
	for name, ctrl := range c.Controllers {
		if matchEps.Intersection(set.NewStrings(ctrl.APIEndpoints...)).IsEmpty() {
			continue
		}

		return &ctrl, name, nil
	}
	return nil, "", errors.NotFoundf("controller with API endpoints %v", endpoints)
}

// CurrentController implements ControllerGetter.CurrentController
func (c *MemStore) CurrentController() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.CurrentControllerName == "" {
		return "", errors.NotFoundf("current controller")
	}
	return c.CurrentControllerName, nil
}

// SetCurrentController implements ControllerUpdater.SetCurrentController
func (c *MemStore) SetCurrentController(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(name); err != nil {
		return err
	}
	if _, ok := c.Controllers[name]; !ok {
		return errors.NotFoundf("controller %s", name)
	}
	c.CurrentControllerName = name
	return nil
}

// AddController implements ControllerUpdater.AddController
func (c *MemStore) AddController(name string, one ControllerDetails) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(name); err != nil {
		return err
	}
	if err := ValidateControllerDetails(one); err != nil {
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
func (c *MemStore) UpdateController(name string, one ControllerDetails) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(name); err != nil {
		return err
	}
	if err := ValidateControllerDetails(one); err != nil {
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
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(name); err != nil {
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
		delete(c.CookieJars, name)
	}
	return nil
}

// UpdateModel implements ModelUpdater.
func (c *MemStore) UpdateModel(controller, model string, details ModelDetails) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(controller); err != nil {
		return err
	}
	if err := ValidateModel(model, details); err != nil {
		return err
	}
	controllerModels, ok := c.Models[controller]
	if !ok {
		controllerModels = &ControllerModels{
			Models: make(map[string]ModelDetails),
		}
		c.Models[controller] = controllerModels
	}
	controllerModels.Models[model] = details
	return nil
}

// SetModels implements ModelUpdater.
func (c *MemStore) SetModels(controller string, models map[string]ModelDetails) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(controller); err != nil {
		return err
	}
	for modelName, details := range models {
		if err := ValidateModel(modelName, details); err != nil {
			return errors.Trace(err)
		}
	}

	controllerModels, ok := c.Models[controller]
	if !ok {
		controllerModels = &ControllerModels{
			Models: make(map[string]ModelDetails),
		}
		c.Models[controller] = controllerModels
	}
	controllerModels.Models = models
	if _, ok := models[controllerModels.CurrentModel]; !ok {
		controllerModels.CurrentModel = ""
	}
	return nil
}

// SetCurrentModel implements ModelUpdater.
func (c *MemStore) SetCurrentModel(controllerName, modelName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	controllerModels, ok := c.Models[controllerName]
	if !ok {
		return errors.NotFoundf("model %s:%s", controllerName, modelName)
	}
	if modelName == "" {
		// We just want to reset
		controllerModels.CurrentModel = ""
		return nil
	}

	if err := ValidateModelName(modelName); err != nil {
		return errors.Trace(err)
	}
	if _, ok := controllerModels.Models[modelName]; !ok {
		return errors.NotFoundf("model %s:%s", controllerName, modelName)
	}
	controllerModels.CurrentModel = modelName
	return nil
}

// RemoveModel implements ModelRemover.
func (c *MemStore) RemoveModel(controller, model string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(controller); err != nil {
		return err
	}
	if err := ValidateModelName(model); err != nil {
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
	return nil
}

// AllModels implements ModelGetter.
func (c *MemStore) AllModels(controller string) (map[string]ModelDetails, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(controller); err != nil {
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
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(controller); err != nil {
		return "", err
	}
	controllerModels, ok := c.Models[controller]
	if !ok {
		return "", errors.NotFoundf("current model for controller %s", controller)
	}
	if controllerModels.CurrentModel == "" {
		return "", errors.NotFoundf("current model for controller %s", controller)
	}
	return controllerModels.CurrentModel, nil
}

// ModelByName implements ModelGetter.
func (c *MemStore) ModelByName(controller, model string) (*ModelDetails, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(controller); err != nil {
		return nil, err
	}
	if err := ValidateModelName(model); err != nil {
		return nil, err
	}
	controllerModels, ok := c.Models[controller]
	if !ok {
		return nil, errors.NotFoundf("model %s:%s", controller, model)
	}
	details, ok := controllerModels.Models[model]
	if !ok {
		return nil, errors.NotFoundf("model %s:%s", controller, model)
	}
	return &details, nil
}

// UpdateAccount implements AccountUpdater.
func (c *MemStore) UpdateAccount(controllerName string, details AccountDetails) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	oldDetails, ok := c.Accounts[controllerName]
	if ok && c.ImmutableAccount {
		return nil
	}

	if err := ValidateControllerName(controllerName); err != nil {
		return err
	}
	if err := ValidateAccountDetails(details); err != nil {
		return err
	}
	// Only update last known access if it has a value.
	if details.LastKnownAccess == "" {
		details.LastKnownAccess = oldDetails.LastKnownAccess
	}
	c.Accounts[controllerName] = details
	return nil
}

// AccountDetails implements AccountGetter.
func (c *MemStore) AccountDetails(controllerName string) (*AccountDetails, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(controllerName); err != nil {
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
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(controllerName); err != nil {
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
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(details.AuthCredentials) > 0 {
		c.Credentials[cloudName] = details
	} else {
		delete(c.Credentials, cloudName)
	}
	return nil
}

// CredentialForCloud implements CredentialsGetter.
func (c *MemStore) CredentialForCloud(cloudName string) (*cloud.CloudCredential, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if result, ok := c.Credentials[cloudName]; ok {
		return &result, nil
	}
	return nil, errors.NotFoundf("credentials for cloud %s", cloudName)
}

// AllCredentials implements CredentialsGetter.
func (c *MemStore) AllCredentials() (map[string]cloud.CloudCredential, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make(map[string]cloud.CloudCredential)
	for k, v := range c.Credentials {
		result[k] = v
	}
	return result, nil
}

// UpdateBootstrapConfig implements BootstrapConfigUpdater.
func (c *MemStore) UpdateBootstrapConfig(controllerName string, cfg BootstrapConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(controllerName); err != nil {
		return err
	}
	if err := ValidateBootstrapConfig(cfg); err != nil {
		return err
	}
	c.BootstrapConfig[controllerName] = cfg
	return nil

}

// BootstrapConfigForController implements BootstrapConfigGetter.
func (c *MemStore) BootstrapConfigForController(controllerName string) (*BootstrapConfig, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cfg, ok := c.BootstrapConfig[controllerName]; ok {
		// TODO(stickupkid): This can be removed once series has been removed.
		// This is here to keep us honest with the tests, although not required.
		if key, ok := cfg.Config[config.DefaultBaseKey]; ok {
			if key == nil || key == "" {
				cfg.Config[config.DefaultSeriesKey] = ""
			} else {
				base, err := corebase.ParseBaseFromString(key.(string))
				if err != nil {
					return nil, errors.Trace(err)
				}

				s, err := corebase.GetSeriesFromBase(base)
				if err != nil {
					return nil, errors.Trace(err)
				}
				cfg.Config[config.DefaultSeriesKey] = s
			}
		}
		return &cfg, nil
	}
	return nil, errors.NotFoundf("bootstrap config for controller %s", controllerName)
}

func (c *MemStore) CookieJar(controllerName string) (CookieJar, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ValidateControllerName(controllerName); err != nil {
		return nil, errors.Trace(err)
	}
	if jar, ok := c.CookieJars[controllerName]; ok {
		return jar, nil
	}
	jar, err := cookiejar.New(&cookiejar.Options{
		NoPersist: true,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.CookieJars[controllerName] = jar
	return jar, nil
}
