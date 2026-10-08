// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
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

const (
	retryableProxyError        = "etcdserver: leader changed"
	forwardingPortsErrorPrefix = "forwarding ports:"
	maxProxyConnectionAttempts = 3
	proxyConnectionRetryDelay  = time.Second
)

// isRetryableProxyError matches the transient etcd leader-change error.
// Kubernetes returns server-side errors as StatusError. The forwarding ports
// prefix is checked because client-go's port-forwarding path formats the
// server error as "forwarding ports: <error>".
func isRetryableProxyError(err error) bool {
	if err == nil {
		return false
	}

	cause := errors.Cause(err)
	if apierrors.IsInternalError(cause) {
		return strings.Contains(cause.Error(), retryableProxyError)
	}

	return strings.HasPrefix(err.Error(), forwardingPortsErrorPrefix) &&
		strings.Contains(err.Error(), retryableProxyError)
}

func (p *Proxier) Start(ctx context.Context) (err error) {
	defer func() {
		err = errors.Annotate(err, "connecting k8s proxy")
	}()

	err = retry.Call(retry.CallArgs{
		Func: func() error {
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

			if err := tunnel.ForwardPort(ctx); err != nil {
				tunnel.Close()
				return err
			}
			return nil
		},
		IsFatalError: func(err error) bool {
			return !isRetryableProxyError(err)
		},
		NotifyFunc: func(err error, attempt int) {
			logger.Debugf("k8s proxy connection attempt %d failed: %v", attempt, err)
		},
		Attempts: maxProxyConnectionAttempts,
		Delay:    proxyConnectionRetryDelay,
		Clock:    clock.WallClock,
		Stop:     ctx.Done(),
	})

	if err != nil {
		if retry.IsRetryStopped(err) {
			return errors.Trace(ctx.Err())
		}
		if retry.IsAttemptsExceeded(err) {
			err = retry.LastError(err)
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
