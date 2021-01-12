// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"github.com/juju/errors"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/proxy"
)

const (
	proxyConfConfigKey = "config"
	proxyConfTypeKey   = "type"
)

var (
	NewProxierFactory func() (*proxy.Factory, error) = proxy.NewDefaultFactory
)

// ProxyConfWrapper is wrapper around proxier interfaces so that they can be
// serialized to json correctly.
type ProxyConfWrapper struct {
	Proxier proxy.Proxier
}

// MarshalYAML implements marshalling method for yaml. This is so we can make
// sure the proxier type is outputted with the config for later ingestion
func (p *ProxyConfWrapper) MarshalYAML() (interface{}, error) {
	return map[string]interface{}{
		proxyConfTypeKey:   p.Proxier.Type(),
		proxyConfConfigKey: p.Proxier,
	}, nil
}

// UnmarshalYAML ingests a previously outputted proxy config. It uses the proxy
// default factory to try and construct the correct proxy based on type.
func (p *ProxyConfWrapper) UnmarshalYAML(unmarshal func(interface{}) error) error {
	factory, err := NewProxierFactory()
	if err != nil {
		return errors.Annotate(err, "building proxy factory for config")
	}

	proxyConf := struct {
		Type   string    `yaml:"type"`
		Config yaml.Node `yaml:"config"`
	}{}

	err = unmarshal(&proxyConf)
	if err != nil {
		return errors.Annotate(err, "unmarshalling raw proxy config")
	}

	maker, err := factory.MakerForTypeKey(proxyConf.Type)
	if err != nil {
		return errors.Trace(err)
	}

	if err = proxyConf.Config.Decode(maker.Config()); err != nil {
		return errors.Annotatef(err, "decoding config for proxy of type %s", proxyConf.Type)
	}

	p.Proxier, err = maker.Make()
	if err != nil {
		return errors.Annotatef(err, "making proxier for type %s", proxyConf.Type)
	}

	return nil
}
