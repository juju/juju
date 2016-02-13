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

	UpdateModelFunc        func(controller, model string, details jujuclient.ModelDetails) error
	SetCurrentModelFunc    func(controllerName, modelName string) error
	SetControllerModelFunc func(controller, model string) error
	RemoveModelFunc        func(controller, model string) error
	AllModelsFunc          func(controller string) (map[string]jujuclient.ModelDetails, error)
	CurrentModelFunc       func(controller string) (string, error)
	ModelByNameFunc        func(controller, model string) (*jujuclient.ModelDetails, error)

	UpdateAccountFunc     func(controllerName, accountName string, details jujuclient.AccountDetails) error
	SetCurrentAccountFunc func(controllerName, accountName string) error
	AllAccountsFunc       func(controllerName string) (map[string]jujuclient.AccountDetails, error)
	CurrentAccountFunc    func(controllerName string) (string, error)
	AccountByNameFunc     func(controllerName, accountName string) (*jujuclient.AccountDetails, error)
	RemoveAccountFunc     func(controllerName, accountName string) error
}

func NewStubStore() *StubStore {
	allControllers := func() (map[string]jujuclient.ControllerDetails, error) {
		return nil, nil
	}
	controllerByName := func(name string) (*jujuclient.ControllerDetails, error) {
		return nil, nil
	}
	updateController := func(name string, one jujuclient.ControllerDetails) error {
		return nil
	}
	removeController := func(name string) error {
		return nil
	}

	updateModel := func(controller, model string, details jujuclient.ModelDetails) error {
		return nil
	}
	setCurrentModel := func(controllerName, modelName string) error {
		return nil
	}
	setControllerModel := func(controller, model string) error {
		return nil
	}
	removeModel := func(controller, model string) error {
		return nil
	}
	allModels := func(controller string) (map[string]jujuclient.ModelDetails, error) {
		return nil, nil
	}
	currentModel := func(controller string) (string, error) {
		return "", nil
	}
	modelByName := func(controller, model string) (*jujuclient.ModelDetails, error) {
		return nil, nil
	}

	updateAccount := func(controllerName, accountName string, details jujuclient.AccountDetails) error {
		return nil
	}
	setCurrentAccount := func(controllerName, accountName string) error {
		return nil
	}
	allAccounts := func(controllerName string) (map[string]jujuclient.AccountDetails, error) {
		return nil, nil
	}
	currentAccount := func(controllerName string) (string, error) {
		return "", nil
	}
	accountByName := func(controllerName, accountName string) (*jujuclient.AccountDetails, error) {
		return nil, nil
	}
	removeAccount := func(controllerName, accountName string) error {
		return nil
	}

	return &StubStore{
		Stub:                 &testing.Stub{},
		AllControllersFunc:   allControllers,
		ControllerByNameFunc: controllerByName,
		UpdateControllerFunc: updateController,
		RemoveControllerFunc: removeController,

		UpdateModelFunc:        updateModel,
		SetCurrentModelFunc:    setCurrentModel,
		SetControllerModelFunc: setControllerModel,
		RemoveModelFunc:        removeModel,
		AllModelsFunc:          allModels,
		CurrentModelFunc:       currentModel,
		ModelByNameFunc:        modelByName,

		UpdateAccountFunc:     updateAccount,
		SetCurrentAccountFunc: setCurrentAccount,
		AllAccountsFunc:       allAccounts,
		CurrentAccountFunc:    currentAccount,
		AccountByNameFunc:     accountByName,
		RemoveAccountFunc:     removeAccount,
	}
}

// AllControllers implements ControllersGetter.AllControllers
func (c *StubStore) AllControllers() (map[string]jujuclient.ControllerDetails, error) {
	c.AddCall("AllControllers")
	return c.AllControllersFunc()
}

// ControllerByName implements ControllersGetter.ControllerByName
func (c *StubStore) ControllerByName(name string) (*jujuclient.ControllerDetails, error) {
	c.AddCall("ControllerByName", name)
	return c.ControllerByNameFunc(name)
}

// UpdateController implements ControllersUpdater.UpdateController
func (c *StubStore) UpdateController(name string, one jujuclient.ControllerDetails) error {
	c.AddCall("UpdateController", name, one)
	return c.UpdateControllerFunc(name, one)
}

// RemoveController implements ControllersRemover.RemoveController
func (c *StubStore) RemoveController(name string) error {
	c.AddCall("RemoveController", name)
	return c.RemoveControllerFunc(name)
}

// UpdateModel implements ModelUpdater.
func (c *StubStore) UpdateModel(controller, model string, details jujuclient.ModelDetails) error {
	c.AddCall("UpdateModel", controller, model, details)
	return c.UpdateModelFunc(controller, model, details)
}

// SetCurrentModel implements ModelUpdater.
func (c *StubStore) SetCurrentModel(controllerName, modelName string) error {
	c.AddCall("SetCurrentModel", controllerName, modelName)
	return c.SetCurrentModelFunc(controllerName, modelName)
}

// RemoveModel implements ModelRemover.
func (c *StubStore) RemoveModel(controller, model string) error {
	c.AddCall("RemoveModel", controller, model)
	return c.RemoveModelFunc(controller, model)
}

// AllModels implements ModelGetter.
func (c *StubStore) AllModels(controller string) (map[string]jujuclient.ModelDetails, error) {
	c.AddCall("AllModels", controller)
	return c.AllModelsFunc(controller)
}

// CurrentModel implements ModelGetter.
func (c *StubStore) CurrentModel(controller string) (string, error) {
	c.AddCall("CurrentModel", controller)
	return c.CurrentModelFunc(controller)
}

// ModelByName implements ModelGetter.
func (c *StubStore) ModelByName(controller, model string) (*jujuclient.ModelDetails, error) {
	c.AddCall("ModelByName", controller, model)
	return c.ModelByNameFunc(controller, model)
}

// UpdateAccount implements AccountUpdater.
func (c *StubStore) UpdateAccount(controllerName, accountName string, details jujuclient.AccountDetails) error {
	c.AddCall("UpdateAccount", controllerName, accountName, details)
	return c.UpdateAccountFunc(controllerName, accountName, details)
}

// SetCurrentAccount implements AccountUpdater.
func (c *StubStore) SetCurrentAccount(controllerName, accountName string) error {
	c.AddCall("SetCurrentAccount", controllerName, accountName)
	return c.SetCurrentAccountFunc(controllerName, accountName)
}

// AllAccounts implements AccountGetter.
func (c *StubStore) AllAccounts(controllerName string) (map[string]jujuclient.AccountDetails, error) {
	c.AddCall("AllAccounts", controllerName)
	return c.AllAccountsFunc(controllerName)
}

// CurrentAccount implements AccountGetter.
func (c *StubStore) CurrentAccount(controllerName string) (string, error) {
	c.AddCall("CurrentAccount", controllerName)
	return c.CurrentAccountFunc(controllerName)
}

// AccountByName implements AccountGetter.
func (c *StubStore) AccountByName(controllerName, accountName string) (*jujuclient.AccountDetails, error) {
	c.AddCall("AccountByName", controllerName, accountName)
	return c.AccountByNameFunc(controllerName, accountName)
}

// RemoveAccount implements AccountRemover.
func (c *StubStore) RemoveAccount(controllerName, accountName string) error {
	c.AddCall("RemoveAccount", controllerName, accountName)
	return c.RemoveAccountFunc(controllerName, accountName)
}
