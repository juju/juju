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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/caas/kubernetes"
	proxyerrors "github.com/juju/juju/proxy/errors"
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

const retryableProxyError = "etcdserver: leader changed"

// isRetryableProxyError deliberately matches the current etcd leader-change
// message because this is the transient error addressed by #21324.
func isRetryableProxyError(err error) bool {
	if !strings.Contains(err.Error(), retryableProxyError) {
		return false
	}

	if apierrors.IsInternalError(err) {
		return true
	}

	if _, ok := err.(*url.Error); ok {
		return true
	}

	return strings.Contains(err.Error(), "forwarding ports:")
}

// retryProxyConnection retries only the transient Kubernetes leader-change
// error. Each attempt creates a fresh tunnel because tunnels are single-use
// for port forwarding. Three attempts with a one-second delay are sufficient
// to allow a transient etcd leader change to settle without adding significant
// delay to normal failures.
func retryProxyConnection(ctx context.Context, connect func(context.Context) error) error {
	const (
		maxAttempts = 3
		retryDelay  = time.Second
	)
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = connect(ctx)
		if err == nil {
			return nil
		}

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

	return errors.Trace(err)
}

func (p *Proxier) Start(ctx context.Context) (err error) {
	defer func() {
		err = errors.Annotate(err, "connecting k8s proxy")
	}()

	err = retryProxyConnection(ctx, func(ctx context.Context) error {
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

		err := tunnel.ForwardPort(ctx)
		if err != nil {
			tunnel.Close()
		}
		return err
	})
	if err != nil {
		urlErr, ok := errors.Cause(err).(*url.Error)
		if !ok {
			return errors.Trace(err)
		}
		if _, ok = urlErr.Err.(*net.OpError); !ok {
			return errors.Trace(err)
		}
		return proxyerrors.NewProxyConnectError(err, p.Type())
	}

	return nil
}

func (p *Proxier) Stop() {
	if p.tunnel != nil {
		p.tunnel.Close()
	}
}

func (p *Proxier) Type() string {
	return ProxierTypeKey
}
