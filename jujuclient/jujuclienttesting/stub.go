// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclienttesting

import (
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient"
)

type StubStore struct {
	*testhelpers.Stub

	AllControllersFunc           func() (map[string]jujuclient.ControllerDetails, error)
	ControllerByNameFunc         func(name string) (*jujuclient.ControllerDetails, error)
	ControllerByAPIEndpointsFunc func(endpoints ...string) (*jujuclient.ControllerDetails, string, error)
	AddControllerFunc            func(name string, one jujuclient.ControllerDetails) error
	UpdateControllerFunc         func(name string, one jujuclient.ControllerDetails) error
	RemoveControllerFunc         func(name string) error
	SetCurrentControllerFunc     func(name string) error
	CurrentControllerFunc        func() (string, error)
	PreviousControllerFunc       func() (string, bool, error)

	UpdateModelFunc     func(controller, model string, details jujuclient.ModelDetails) error
	SetCurrentModelFunc func(controller, model string) error
	RemoveModelFunc     func(controller, model string) error
	AllModelsFunc       func(controller string) (map[string]jujuclient.ModelDetails, error)
	CurrentModelFunc    func(controller string) (string, error)
	PreviousModelFunc   func(controller string) (string, error)
	ModelByNameFunc     func(controller, model string) (*jujuclient.ModelDetails, error)
	SetModelsFunc       func(controllerName string, models map[string]jujuclient.ModelDetails) error

	UpdateAccountFunc  func(controllerName string, details jujuclient.AccountDetails) error
	AccountDetailsFunc func(controllerName string) (*jujuclient.AccountDetails, error)
	RemoveAccountFunc  func(controllerName string) error

	CredentialForCloudFunc func(string) (*cloud.CloudCredential, error)
	AllCredentialsFunc     func() (map[string]cloud.CloudCredential, error)
	UpdateCredentialFunc   func(cloudName string, details cloud.CloudCredential) error

	BootstrapConfigForControllerFunc func(controllerName string) (*jujuclient.BootstrapConfig, error)
	UpdateBootstrapConfigFunc        func(controllerName string, cfg jujuclient.BootstrapConfig) error

	CookieJarFunc func(controllerName string) (jujuclient.CookieJar, error)
}

func NewStubStore() *StubStore {
	result := &StubStore{
		Stub: &testhelpers.Stub{},
	}
	result.AllControllersFunc = func() (map[string]jujuclient.ControllerDetails, error) {
		return nil, result.Stub.NextErr()
	}
	result.ControllerByNameFunc = func(name string) (*jujuclient.ControllerDetails, error) {
		return nil, result.Stub.NextErr()
	}
	result.ControllerByAPIEndpointsFunc = func(endpoints ...string) (*jujuclient.ControllerDetails, string, error) {
		return nil, "", result.Stub.NextErr()
	}
	result.AddControllerFunc = func(name string, one jujuclient.ControllerDetails) error {
		return result.Stub.NextErr()
	}
	result.UpdateControllerFunc = func(name string, one jujuclient.ControllerDetails) error {
		return result.Stub.NextErr()
	}
	result.RemoveControllerFunc = func(name string) error {
		return result.Stub.NextErr()
	}
	result.SetCurrentControllerFunc = func(name string) error {
		return result.Stub.NextErr()
	}
	result.CurrentControllerFunc = func() (string, error) {
		return "", result.Stub.NextErr()
	}
	result.PreviousControllerFunc = func() (string, bool, error) {
		return "", false, result.Stub.NextErr()
	}

	result.UpdateModelFunc = func(controller, model string, details jujuclient.ModelDetails) error {
		return result.Stub.NextErr()
	}
	result.SetCurrentModelFunc = func(controller, model string) error {
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
	result.PreviousModelFunc = func(controller string) (string, error) {
		return "", result.Stub.NextErr()
	}
	result.ModelByNameFunc = func(controller, model string) (*jujuclient.ModelDetails, error) {
		return nil, result.Stub.NextErr()
	}
	result.SetModelsFunc = func(controllerName string, models map[string]jujuclient.ModelDetails) error {
		return result.Stub.NextErr()
	}

	result.UpdateAccountFunc = func(controllerName string, details jujuclient.AccountDetails) error {
		return result.Stub.NextErr()
	}
	result.AccountDetailsFunc = func(controllerName string) (*jujuclient.AccountDetails, error) {
		return nil, result.Stub.NextErr()
	}
	result.RemoveAccountFunc = func(controllerName string) error {
		return result.Stub.NextErr()
	}

	result.CredentialForCloudFunc = func(string) (*cloud.CloudCredential, error) {
		return nil, result.Stub.NextErr()
	}
	result.AllCredentialsFunc = func() (map[string]cloud.CloudCredential, error) {
		return nil, result.Stub.NextErr()
	}
	result.UpdateCredentialFunc = func(cloudName string, details cloud.CloudCredential) error {
		return result.Stub.NextErr()
	}

	result.BootstrapConfigForControllerFunc = func(controllerName string) (*jujuclient.BootstrapConfig, error) {
		return nil, result.Stub.NextErr()
	}
	result.UpdateBootstrapConfigFunc = func(controllerName string, cfg jujuclient.BootstrapConfig) error {
		return result.Stub.NextErr()
	}
	result.CookieJarFunc = func(controllerName string) (jujuclient.CookieJar, error) {
		return nil, result.Stub.NextErr()
	}
	return result
}

// WrapClientStore wraps a ClientStore with a StubStore, where each method calls
// through to the wrapped store. This can be used to override specific
// methods, or just to check which calls have been made.
func WrapClientStore(underlying jujuclient.ClientStore) *StubStore {
	stub := NewStubStore()
	stub.AllControllersFunc = underlying.AllControllers
	stub.ControllerByNameFunc = underlying.ControllerByName
	stub.ControllerByAPIEndpointsFunc = underlying.ControllerByAPIEndpoints
	stub.AddControllerFunc = underlying.AddController
	stub.UpdateControllerFunc = underlying.UpdateController
	stub.RemoveControllerFunc = underlying.RemoveController
	stub.SetCurrentControllerFunc = underlying.SetCurrentController
	stub.CurrentControllerFunc = underlying.CurrentController
	stub.PreviousControllerFunc = underlying.PreviousController
	stub.UpdateModelFunc = underlying.UpdateModel
	stub.SetModelsFunc = underlying.SetModels
	stub.SetCurrentModelFunc = underlying.SetCurrentModel
	stub.RemoveModelFunc = underlying.RemoveModel
	stub.AllModelsFunc = underlying.AllModels
	stub.CurrentModelFunc = underlying.CurrentModel
	stub.PreviousModelFunc = underlying.PreviousModel
	stub.ModelByNameFunc = underlying.ModelByName
	stub.UpdateAccountFunc = underlying.UpdateAccount
	stub.AccountDetailsFunc = underlying.AccountDetails
	stub.RemoveAccountFunc = underlying.RemoveAccount
	stub.BootstrapConfigForControllerFunc = underlying.BootstrapConfigForController
	stub.UpdateBootstrapConfigFunc = underlying.UpdateBootstrapConfig
	stub.CookieJarFunc = underlying.CookieJar
	return stub
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

// ControllerByAPIEndpoints implements ControllersGetter.ControllerByAPIEndpoints
func (c *StubStore) ControllerByAPIEndpoints(endpoints ...string) (*jujuclient.ControllerDetails, string, error) {
	c.MethodCall(c, "ControllerByAPIEndpoints", endpoints)
	return c.ControllerByAPIEndpointsFunc(endpoints...)
}

// AddController implements ControllerUpdater.AddController
func (c *StubStore) AddController(name string, one jujuclient.ControllerDetails) error {
	c.MethodCall(c, "AddController", name, one)
	return c.AddControllerFunc(name, one)
}

// UpdateController implements ControllerUpdater.UpdateController
func (c *StubStore) UpdateController(name string, one jujuclient.ControllerDetails) error {
	c.MethodCall(c, "UpdateController", name, one)
	return c.UpdateControllerFunc(name, one)
}

// RemoveController implements ControllersRemover.RemoveController
func (c *StubStore) RemoveController(name string) error {
	c.MethodCall(c, "RemoveController", name)
	return c.RemoveControllerFunc(name)
}

// SetCurrentController implements ControllerUpdater.SetCurrentController.
func (c *StubStore) SetCurrentController(name string) error {
	c.MethodCall(c, "SetCurrentController", name)
	return c.SetCurrentControllerFunc(name)
}

// CurrentController implements ControllersGetter.CurrentController.
func (c *StubStore) CurrentController() (string, error) {
	c.MethodCall(c, "CurrentController")
	return c.CurrentControllerFunc()
}

// PreviousController implements ControllersGetter.PreviousController.
func (c *StubStore) PreviousController() (string, bool, error) {
	c.MethodCall(c, "PreviousController")
	return c.PreviousControllerFunc()
}

// UpdateModel implements ModelUpdater.
func (c *StubStore) UpdateModel(controller, model string, details jujuclient.ModelDetails) error {
	c.MethodCall(c, "UpdateModel", controller, model, details)
	return c.UpdateModelFunc(controller, model, details)
}

// SetModels implements ModelUpdater.
func (c *StubStore) SetModels(controller string, models map[string]jujuclient.ModelDetails) error {
	c.MethodCall(c, "SetModels", controller, models)
	return c.SetModelsFunc(controller, models)
}

// SetCurrentModel implements ModelUpdater.
func (c *StubStore) SetCurrentModel(controller, model string) error {
	c.MethodCall(c, "SetCurrentModel", controller, model)
	return c.SetCurrentModelFunc(controller, model)
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

// PreviousModel implements ModelGetter.
func (c *StubStore) PreviousModel(controller string) (string, error) {
	c.MethodCall(c, "PreviousModel", controller)
	return c.PreviousModelFunc(controller)
}

// ModelByName implements ModelGetter.
func (c *StubStore) ModelByName(controller, model string) (*jujuclient.ModelDetails, error) {
	c.MethodCall(c, "ModelByName", controller, model)
	return c.ModelByNameFunc(controller, model)
}

// UpdateAccount implements AccountUpdater.
func (c *StubStore) UpdateAccount(controllerName string, details jujuclient.AccountDetails) error {
	c.MethodCall(c, "UpdateAccount", controllerName, details)
	return c.UpdateAccountFunc(controllerName, details)
}

// AccountDetails implements AccountGetter.
func (c *StubStore) AccountDetails(controllerName string) (*jujuclient.AccountDetails, error) {
	c.MethodCall(c, "AccountDetails", controllerName)
	return c.AccountDetailsFunc(controllerName)
}

// RemoveAccount implements AccountRemover.
func (c *StubStore) RemoveAccount(controllerName string) error {
	c.MethodCall(c, "RemoveAccount", controllerName)
	return c.RemoveAccountFunc(controllerName)
}

// CredentialForCloud implements CredentialsGetter.
func (c *StubStore) CredentialForCloud(cloudName string) (*cloud.CloudCredential, error) {
	c.MethodCall(c, "CredentialForCloud", cloudName)
	return c.CredentialForCloudFunc(cloudName)
}

// AllCredentials implements CredentialsGetter.
func (c *StubStore) AllCredentials() (map[string]cloud.CloudCredential, error) {
	c.MethodCall(c, "AllCredentials")
	return c.AllCredentialsFunc()
}

// UpdateCredential implements CredentialsUpdater.
func (c *StubStore) UpdateCredential(cloudName string, details cloud.CloudCredential) error {
	c.MethodCall(c, "UpdateCredential", cloudName, details)
	return c.UpdateCredentialFunc(cloudName, details)
}

// BootstrapConfigForController implements BootstrapConfigGetter.
func (c *StubStore) BootstrapConfigForController(controllerName string) (*jujuclient.BootstrapConfig, error) {
	c.MethodCall(c, "BootstrapConfigForController", controllerName)
	return c.BootstrapConfigForControllerFunc(controllerName)
}

// UpdateBootstrapConfig implements BootstrapConfigUpdater.
func (c *StubStore) UpdateBootstrapConfig(controllerName string, cfg jujuclient.BootstrapConfig) error {
	c.MethodCall(c, "UpdateBootstrapConfig", controllerName, cfg)
	return c.UpdateBootstrapConfigFunc(controllerName, cfg)
}

func (c *StubStore) CookieJar(controllerName string) (jujuclient.CookieJar, error) {
	c.MethodCall(c, "CookieJar", controllerName)
	return c.CookieJarFunc(controllerName)
}
