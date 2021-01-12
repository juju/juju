// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"github.com/juju/errors"

	k8sproxy "github.com/juju/juju/caas/kubernetes/provider/proxy"
)

type Factory struct {
	inventory map[string]FactoryRegister
}

type FactoryRegister struct {
	ConfigFn func() interface{}
	MakerFn  func(interface{}) (Proxier, error)
}

type FactoryMaker struct {
	RawConfig interface{}
	Register  FactoryRegister
}

func (f *FactoryMaker) Config() interface{} {
	if f.RawConfig == nil {
		f.RawConfig = f.Register.ConfigFn()
	}
	return f.RawConfig
}

func (f *FactoryMaker) Make() (Proxier, error) {
	return f.Register.MakerFn(f.RawConfig)
}

func (f *Factory) MakerForTypeKey(typeKey string) (*FactoryMaker, error) {
	var (
		exists   bool
		register FactoryRegister
	)
	if register, exists = f.inventory[typeKey]; !exists {
		return nil, errors.NotFoundf("proxy register for key %s", typeKey)
	}
	return &FactoryMaker{Register: register}, nil
}

func NewDefaultFactory() (*Factory, error) {
	factory := &Factory{
		inventory: make(map[string]FactoryRegister),
	}

	if err := factory.Register(
		k8sproxy.ProxierTypeKey,
		FactoryRegister{
			ConfigFn: func() interface{} { return k8sproxy.NewProxierConfig() },
			MakerFn:  func(c interface{}) (Proxier, error) { return k8sproxy.NewProxierFromRawConfig(c) },
		}); err != nil {
		return factory, err
	}

	return factory, nil
}

func (f *Factory) Register(typeKey string, register FactoryRegister) error {
	if _, exists := f.inventory[typeKey]; exists {
		return errors.AlreadyExistsf("proxy register for key %s", typeKey)
	}
	f.inventory[typeKey] = register
	return nil
}
