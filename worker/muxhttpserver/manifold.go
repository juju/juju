// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package muxhttpserver

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/pki"
)

type ManifoldConfig struct {
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

func (c ManifoldConfig) Start(context dependency.Context) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	serverConfig := DefaultConfig()
	if c.Port != "" {
		serverConfig.Port = c.Port
	}

	if c.AuthorityName == "" {
		return NewServerWithOutTLS(c.Logger, serverConfig)
	}

	var authority pki.Authority
	if err := context.Get(c.AuthorityName, &authority); err != nil {
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
