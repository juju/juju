// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclienttesting

import (
	"github.com/juju/testing"

	"github.com/juju/juju/jujuclient"
)

type StubStore struct {
	*testing.Stub

	AllControllersFunc   func() (map[string]jujuclient.ControllerDetails, error)
	ControllerByNameFunc func(name string) (*jujuclient.ControllerDetails, error)
	UpdateControllerFunc func(name string, one jujuclient.ControllerDetails) error
	RemoveControllerFunc func(name string) error

	UpdateModelFunc     func(controller, model string, details jujuclient.ModelDetails) error
	SetCurrentModelFunc func(controllerName, modelName string) error
	RemoveModelFunc     func(controller, model string) error
	AllModelsFunc       func(controller string) (map[string]jujuclient.ModelDetails, error)
	CurrentModelFunc    func(controller string) (string, error)
	ModelByNameFunc     func(controller, model string) (*jujuclient.ModelDetails, error)

	UpdateAccountFunc     func(controllerName, accountName string, details jujuclient.AccountDetails) error
	SetCurrentAccountFunc func(controllerName, accountName string) error
	AllAccountsFunc       func(controllerName string) (map[string]jujuclient.AccountDetails, error)
	CurrentAccountFunc    func(controllerName string) (string, error)
	AccountByNameFunc     func(controllerName, accountName string) (*jujuclient.AccountDetails, error)
	RemoveAccountFunc     func(controllerName, accountName string) error
}

func NewStubStore() *StubStore {
	result := &StubStore{
		Stub: &testing.Stub{},
	}
	result.AllControllersFunc = func() (map[string]jujuclient.ControllerDetails, error) {
		return nil, result.Stub.NextErr()
	}
	result.ControllerByNameFunc = func(name string) (*jujuclient.ControllerDetails, error) {
		return nil, result.Stub.NextErr()
	}
	result.UpdateControllerFunc = func(name string, one jujuclient.ControllerDetails) error {
		return result.Stub.NextErr()
	}
	result.RemoveControllerFunc = func(name string) error {
		return result.Stub.NextErr()
	}

	result.UpdateModelFunc = func(controller, model string, details jujuclient.ModelDetails) error {
		return result.Stub.NextErr()
	}
	result.SetCurrentModelFunc = func(controllerName, modelName string) error {
		return result.Stub.NextErr()
	}
	result.RemoveModelFunc = func(controller, model string) error {
		return result.Stub.NextErr()
	}
	result.AllModelsFunc = func(controller string) (map[string]jujuclient.ModelDetails, error) {
		return nil, result.Stub.NextErr()
	}
	result.CurrentModelFunc = func(controller string) (string, error) {
		return "", result.Stub.NextErr()
	}
	result.ModelByNameFunc = func(controller, model string) (*jujuclient.ModelDetails, error) {
		return nil, result.Stub.NextErr()
	}

	result.UpdateAccountFunc = func(controllerName, accountName string, details jujuclient.AccountDetails) error {
		return result.Stub.NextErr()
	}
	result.SetCurrentAccountFunc = func(controllerName, accountName string) error {
		return result.Stub.NextErr()
	}
	result.AllAccountsFunc = func(controllerName string) (map[string]jujuclient.AccountDetails, error) {
		return nil, result.Stub.NextErr()
	}
	result.CurrentAccountFunc = func(controllerName string) (string, error) {
		return "", result.Stub.NextErr()
	}
	result.AccountByNameFunc = func(controllerName, accountName string) (*jujuclient.AccountDetails, error) {
		return nil, result.Stub.NextErr()
	}
	result.RemoveAccountFunc = func(controllerName, accountName string) error {
		return result.Stub.NextErr()
	}

	return result
}

// AllControllers implements ControllersGetter.AllControllers
func (c *StubStore) AllControllers() (map[string]jujuclient.ControllerDetails, error) {
	c.MethodCall(c, "AllControllers")
	return c.AllControllersFunc()
}

// ControllerByName implements ControllersGetter.ControllerByName
func (c *StubStore) ControllerByName(name string) (*jujuclient.ControllerDetails, error) {
	c.MethodCall(c, "ControllerByName", name)
	return c.ControllerByNameFunc(name)
}

// UpdateController implements ControllersUpdater.UpdateController
func (c *StubStore) UpdateController(name string, one jujuclient.ControllerDetails) error {
	c.MethodCall(c, "UpdateController", name, one)
	return c.UpdateControllerFunc(name, one)
}

// RemoveController implements ControllersRemover.RemoveController
func (c *StubStore) RemoveController(name string) error {
	c.MethodCall(c, "RemoveController", name)
	return c.RemoveControllerFunc(name)
}

// UpdateModel implements ModelUpdater.
func (c *StubStore) UpdateModel(controller, model string, details jujuclient.ModelDetails) error {
	c.MethodCall(c, "UpdateModel", controller, model, details)
	return c.UpdateModelFunc(controller, model, details)
}

// SetCurrentModel implements ModelUpdater.
func (c *StubStore) SetCurrentModel(controllerName, modelName string) error {
	c.MethodCall(c, "SetCurrentModel", controllerName, modelName)
	return c.SetCurrentModelFunc(controllerName, modelName)
}

// RemoveModel implements ModelRemover.
func (c *StubStore) RemoveModel(controller, model string) error {
	c.MethodCall(c, "RemoveModel", controller, model)
	return c.RemoveModelFunc(controller, model)
}

// AllModels implements ModelGetter.
func (c *StubStore) AllModels(controller string) (map[string]jujuclient.ModelDetails, error) {
	c.MethodCall(c, "AllModels", controller)
	return c.AllModelsFunc(controller)
}

// CurrentModel implements ModelGetter.
func (c *StubStore) CurrentModel(controller string) (string, error) {
	c.MethodCall(c, "CurrentModel", controller)
	return c.CurrentModelFunc(controller)
}

// ModelByName implements ModelGetter.
func (c *StubStore) ModelByName(controller, model string) (*jujuclient.ModelDetails, error) {
	c.MethodCall(c, "ModelByName", controller, model)
	return c.ModelByNameFunc(controller, model)
}

// UpdateAccount implements AccountUpdater.
func (c *StubStore) UpdateAccount(controllerName, accountName string, details jujuclient.AccountDetails) error {
	c.MethodCall(c, "UpdateAccount", controllerName, accountName, details)
	return c.UpdateAccountFunc(controllerName, accountName, details)
}

// SetCurrentAccount implements AccountUpdater.
func (c *StubStore) SetCurrentAccount(controllerName, accountName string) error {
	c.MethodCall(c, "SetCurrentAccount", controllerName, accountName)
	return c.SetCurrentAccountFunc(controllerName, accountName)
}

// AllAccounts implements AccountGetter.
func (c *StubStore) AllAccounts(controllerName string) (map[string]jujuclient.AccountDetails, error) {
	c.MethodCall(c, "AllAccounts", controllerName)
	return c.AllAccountsFunc(controllerName)
}

// CurrentAccount implements AccountGetter.
func (c *StubStore) CurrentAccount(controllerName string) (string, error) {
	c.MethodCall(c, "CurrentAccount", controllerName)
	return c.CurrentAccountFunc(controllerName)
}

// AccountByName implements AccountGetter.
func (c *StubStore) AccountByName(controllerName, accountName string) (*jujuclient.AccountDetails, error) {
	c.MethodCall(c, "AccountByName", controllerName, accountName)
	return c.AccountByNameFunc(controllerName, accountName)
}

// RemoveAccount implements AccountRemover.
func (c *StubStore) RemoveAccount(controllerName, accountName string) error {
	c.MethodCall(c, "RemoveAccount", controllerName, accountName)
	return c.RemoveAccountFunc(controllerName, accountName)
}
