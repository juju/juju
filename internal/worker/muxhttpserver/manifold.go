// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package muxhttpserver

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/internal/pki"
)

type ManifoldConfig struct {
	// Address is the host portion to use to net.Dial
	Address string

	AuthorityName string
	Logger        Logger
	Port          string
}

func Manifold(config ManifoldConfig) dependency.Manifold {
	inputs := []string{}

	if config.AuthorityName != "" {
		inputs = append(inputs, config.AuthorityName)
	}

	return dependency.Manifold{
		Inputs: inputs,
		Output: manifoldOutput,
		Start:  config.Start,
	}
}

func manifoldOutput(in worker.Worker, out interface{}) error {
	inServer, ok := in.(*Server)
	if !ok {
		return errors.Errorf("expected Server, got %T", in)
	}

	switch result := out.(type) {
	case **apiserverhttp.Mux:
		*result = inServer.Mux
	case *ServerInfo:
		*result = inServer.Info()
	default:
		return errors.Errorf("expected Mapper, got %T", out)
	}
	return nil
}

func (c ManifoldConfig) Start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	serverConfig := DefaultConfig()
	if c.Address != "" {
		serverConfig.Address = c.Address
	}

	if c.Port != "" {
		serverConfig.Port = c.Port
	}

	if c.AuthorityName == "" {
		return NewServerWithOutTLS(c.Logger, serverConfig)
	}

	var authority pki.Authority
	if err := getter.Get(c.AuthorityName, &authority); err != nil {
		return nil, errors.Trace(err)
	}

	return NewServer(authority, c.Logger, serverConfig)
}

func (c ManifoldConfig) Validate() error {
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}
