// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"io/ioutil"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
)

// EndpointProvider defines the method used by the WriteServerFile
// in order
type EndpointProvider interface {
	ConnectionEndpoint() (configstore.APIEndpoint, error)
}

func writeServerFile(endpointProvider EndpointProvider, ctx *cmd.Context, username, password, outPath string) error {
	endpoint, err := endpointProvider.ConnectionEndpoint()
	if err != nil {
		return errors.Trace(err)
	}

	outputInfo := envcmd.ServerFile{
		Username:  username,
		Password:  password,
		Addresses: endpoint.Addresses,
		CACert:    endpoint.CACert,
	}
	yaml, err := cmd.FormatYaml(outputInfo)
	if err != nil {
		return errors.Trace(err)
	}
	if err := ioutil.WriteFile(outPath, yaml, 0644); err != nil {
		return errors.Trace(err)
	}
	ctx.Infof("server file written to %s", outPath)
	return nil
}
