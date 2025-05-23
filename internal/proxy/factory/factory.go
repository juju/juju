// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory

import (
	"github.com/juju/errors"
	"github.com/mitchellh/mapstructure"

	k8sproxy "github.com/juju/juju/internal/provider/kubernetes/proxy"
	"github.com/juju/juju/internal/proxy"
)

// Factory provides a mechanism for building various type of proxy based on
// their unique type key. This is primarily developed for use when serialising
// proxy connection information to and from yaml files for Juju.
type Factory struct {
	inventory map[string]FactoryRegister
}

// FactoryRegister describe registration details for building a new proxy.
type FactoryRegister struct {
	ConfigFn func() interface{}
	MakerFn  func(interface{}) (proxy.Proxier, error)
}

// FactoryMaker is shell object used for facilitating the making of a proxy
// object
type FactoryMaker struct {
	RawConfig interface{}
	Register  FactoryRegister
}

// Config returns a raw config for a given proxy type that can be used to
// unmarshal against.
func (f *FactoryMaker) Config() interface{} {
	if f.RawConfig == nil {
		f.RawConfig = f.Register.ConfigFn()
	}
	return f.RawConfig
}

// Make attempts to make the proxy from the filled config.
func (f *FactoryMaker) Make() (proxy.Proxier, error) {
	return f.Register.MakerFn(f.RawConfig)
}

// MakerForTypeKey provides a new factory maker for the given type key if one
// has been registered.
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

// NewDefaultFactory constructs a factory with all the known proxy
// implementations in juju registered.
func NewDefaultFactory() (*Factory, error) {
	factory := NewFactory()

	if err := factory.Register(
		k8sproxy.ProxierTypeKey,
		FactoryRegister{
			ConfigFn: func() interface{} { return k8sproxy.NewProxierConfig() },
			MakerFn:  func(c interface{}) (proxy.Proxier, error) { return k8sproxy.NewProxierFromRawConfig(c) },
		}); err != nil {
		return factory, err
	}

	return factory, nil
}

// NewFactory creates a new empty factory
func NewFactory() *Factory {
	return &Factory{
		inventory: make(map[string]FactoryRegister),
	}
}

// ProxierFromConfig is a utility function for making a proxier from this
// factory using raw config data within in a map[string]interface{}. The type
// key cannot be an empty string.
func (f *Factory) ProxierFromConfig(typeKey string, config map[string]interface{}) (proxy.Proxier, error) {
	if typeKey == "" {
		return nil, errors.NotValidf("type key for proxier cannot be empty")
	}

	maker, err := f.MakerForTypeKey(typeKey)
	if err != nil {
		return nil, err
	}

	if err := mapstructure.Decode(config, maker.Config()); err != nil {
		return nil, errors.Annotatef(err, "decoding config  for proxier type %q", typeKey)
	}

	return maker.Make()
}

// Register registers a new proxier type and creationg methods to this factory
func (f *Factory) Register(typeKey string, register FactoryRegister) error {
	if _, exists := f.inventory[typeKey]; exists {
		return errors.AlreadyExistsf("proxy register for key %s", typeKey)
	}
	f.inventory[typeKey] = register
	return nil
}
