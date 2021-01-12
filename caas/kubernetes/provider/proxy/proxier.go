// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"github.com/juju/errors"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/caas/kubernetes"
)

type Proxier struct {
	config     ProxierConfig
	tunnel     *kubernetes.Tunnel
	restConfig rest.Config
}

type ProxierConfig struct {
	APIHost             string `yaml:"api-host"`
	CAData              string `yaml:"ca-cert"`
	Namespace           string `yaml:"namespace"`
	RemotePort          string `yaml:"remove-port"`
	Service             string `yaml:"service"`
	ServiceAccountToken string `yaml:"service-account-token"`
}

const (
	ProxierTypeKey = "kubernetes-port-forward"
)

func NewProxier(config ProxierConfig) *Proxier {
	return &Proxier{
		config: config,
		restConfig: rest.Config{
			BearerToken: config.ServiceAccountToken,
			Host:        config.APIHost,
			TLSClientConfig: rest.TLSClientConfig{
				CAData: []byte(config.CAData),
			},
		},
	}
}

func NewProxierConfig() *ProxierConfig {
	return &ProxierConfig{}
}

func NewProxierFromRawConfig(rawConf interface{}) (*Proxier, error) {
	conf, valid := rawConf.(*ProxierConfig)
	if !valid {
		return nil, errors.NewNotValid(nil, "config is not of type *ProxierConfig")
	}

	return NewProxier(*conf), nil
}

func (p *Proxier) MarshalYAML() (interface{}, error) {
	return &p.config, nil
}

func (p *Proxier) Port() string {
	return p.tunnel.LocalPort
}

func (p *Proxier) Start() error {
	tunnel, err := kubernetes.NewTunnelForConfig(
		&p.restConfig,
		kubernetes.TunnelKindServices,
		p.config.Namespace,
		p.config.Service,
		p.config.RemotePort,
	)

	if err != nil {
		return errors.Trace(err)
	}
	p.tunnel = tunnel

	return p.tunnel.ForwardPort()
}

func (p *Proxier) Type() string {
	return ProxierTypeKey
}
