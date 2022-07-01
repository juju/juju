// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"net"
	"net/url"

	"github.com/juju/errors"
	"github.com/mitchellh/mapstructure"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/v3/caas/kubernetes"
	proxyerrors "github.com/juju/juju/v3/proxy/errors"
)

type Proxier struct {
	config     ProxierConfig
	tunnel     *kubernetes.Tunnel
	restConfig rest.Config
}

type ProxierConfig struct {
	APIHost             string `yaml:"api-host" mapstructure:"api-host"`
	CAData              string `yaml:"ca-cert" mapstructure:"ca-cert"`
	Namespace           string `yaml:"namespace" mapstructure:"namespace"`
	RemotePort          string `yaml:"remote-port" mapstructure:"remote-port"`
	Service             string `yaml:"service" mapstructure:"service"`
	ServiceAccountToken string `yaml:"service-account-token" mapstructure:"service-account-token"`
}

const (
	ProxierTypeKey = "kubernetes-port-forward"
)

func (p *Proxier) Host() string {
	return "localhost"
}

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

// SetAPIHost updates the proxy info to use a different host address.
func (p *Proxier) SetAPIHost(host string) {
	p.restConfig.Host = host
	p.config.APIHost = host
}

// RawConfig implements Proxier RawConfig interface.
func (p *Proxier) RawConfig() (map[string]interface{}, error) {
	rval := map[string]interface{}{}
	err := mapstructure.Decode(&p.config, &rval)
	return rval, errors.Trace(err)
}

// MarshalYAML implements the yaml Marshaler interface
func (p *Proxier) MarshalYAML() (interface{}, error) {
	return &p.config, nil
}

func (p *Proxier) Port() string {
	return p.tunnel.LocalPort
}

func (p *Proxier) Start() (err error) {
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

	defer func() {
		err = errors.Annotate(err, "connecting k8s proxy")
	}()
	err = p.tunnel.ForwardPort()
	urlErr, ok := errors.Cause(err).(*url.Error)
	if !ok {
		return err
	}
	if _, ok = urlErr.Err.(*net.OpError); !ok {
		return err
	}
	return proxyerrors.NewProxyConnectError(err, p.Type())
}

func (p *Proxier) Stop() {
	if p.tunnel != nil {
		p.tunnel.Close()
	}
}

func (p *Proxier) Type() string {
	return ProxierTypeKey
}
