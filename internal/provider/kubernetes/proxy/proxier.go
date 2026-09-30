// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/mitchellh/mapstructure"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/caas/kubernetes"
	proxyerrors "github.com/juju/juju/internal/proxy/errors"
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
	return "127.0.0.1"
}

func NewProxier(config ProxierConfig) *Proxier {
	p := &Proxier{config: config}
	p.updateRESTConfig()
	return p
}

// Insecure sets the proxy to be insecure.
func (p *Proxier) Insecure() {
	p.config.CAData = ""
	p.updateRESTConfig()
}

func (p *Proxier) updateRESTConfig() {
	restConfig := rest.Config{
		BearerToken:     p.config.ServiceAccountToken,
		Host:            p.config.APIHost,
		TLSClientConfig: rest.TLSClientConfig{},
	}
	if p.config.CAData != "" {
		restConfig.TLSClientConfig.CAData = []byte(p.config.CAData)
	} else {
		restConfig.TLSClientConfig.Insecure = true
	}
	p.restConfig = restConfig
}

func NewProxierConfig() *ProxierConfig {
	return &ProxierConfig{}
}

func NewProxierFromRawConfig(rawConf any) (*Proxier, error) {
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
func (p *Proxier) RawConfig() (map[string]any, error) {
	rval := map[string]any{}
	err := mapstructure.Decode(&p.config, &rval)
	return rval, errors.Trace(err)
}

// MarshalYAML implements the yaml Marshaler interface
func (p *Proxier) MarshalYAML() (any, error) {
	return &p.config, nil
}

func (p *Proxier) Port() string {
	return p.tunnel.LocalPort
}

// ProxyError reports asynchronous port-forwarding errors observed after the
// tunnel was reported as ready.
func (p *Proxier) ProxyError() error {
	if p.tunnel == nil {
		return nil
	}
	return p.tunnel.ForwardError()
}

const retryableProxyError = "etcdserver: leader changed"

func isRetryableProxyError(err error) bool {
	return strings.Contains(err.Error(), retryableProxyError)
}

func (p *Proxier) Start(ctx context.Context) (err error) {
	const (
		maxAttempts = 3
		retryDelay  = time.Second
	)

	defer func() {
		err = errors.Annotate(err, "connecting k8s proxy")
	}()

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		tunnel, tunnelErr := kubernetes.NewTunnelForConfig(
			&p.restConfig,
			kubernetes.TunnelKindServices,
			p.config.Namespace,
			p.config.Service,
			p.config.RemotePort,
		)
		if tunnelErr != nil {
			return errors.Trace(tunnelErr)
		}

		p.tunnel = tunnel

		err = tunnel.ForwardPort(ctx)
		if err == nil {
			return nil
		}

		tunnel.Close()

		if !isRetryableProxyError(err) {
			return errors.Trace(err)
		}

		if attempt == maxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return errors.Trace(ctx.Err())
		case <-time.After(retryDelay):
		}
	}

	urlErr, ok := errors.Cause(err).(*url.Error)
	if !ok {
		return errors.Trace(err)
	}
	if _, ok = urlErr.Err.(*net.OpError); !ok {
		return errors.Trace(err)
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
