// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"github.com/juju/errors"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/internal/proxy"
	proxyfactory "github.com/juju/juju/internal/proxy/factory"
)

// For testing purposes.
var NewProxierFactory = newProxierFactory

func newProxierFactory() (ProxyFactory, error) {
	return proxyfactory.NewDefaultFactory()
}

// ProxyFactory defines the interface for a factory that can create a proxy.
type ProxyFactory interface {
	ProxierFromConfig(string, map[string]interface{}) (proxy.Proxier, error)
}

// ProxyConfWrapper is wrapper around proxier interfaces so that they can be
// serialized to json correctly.
type ProxyConfWrapper struct {
	Proxier proxy.Proxier
}

// MarshalYAML implements marshalling method for yaml. This is so we can make
// sure the proxier type is outputted with the config for later ingestion
func (p *ProxyConfWrapper) MarshalYAML() (interface{}, error) {
	return proxyConfMarshaler{
		Type: p.Proxier.Type(), Config: p.Proxier,
	}, nil
}

type proxyConfMarshaler struct {
	Type   string         `yaml:"type"`
	Config yaml.Marshaler `yaml:"config"`
}

type proxyConfUnmarshaler struct {
	Type   string                 `yaml:"type"`
	Config map[string]interface{} `yaml:"config"`
}

// UnmarshalYAML ingests a previously outputted proxy config. It uses the proxy
// default factory to try and construct the correct proxy based on type.
func (p *ProxyConfWrapper) UnmarshalYAML(unmarshal func(interface{}) error) error {
	factory, err := NewProxierFactory()
	if err != nil {
		return errors.Annotate(err, "building proxy factory for config")
	}

	var pc proxyConfUnmarshaler
	err = unmarshal(&pc)
	if err != nil {
		return errors.Annotate(err, "unmarshalling raw proxy config")
	}
	p.Proxier, err = factory.ProxierFromConfig(pc.Type, pc.Config)
	if err != nil {
		return errors.Annotatef(err, "cannot make proxier for type %s", pc.Type)
	}
	return nil
}
