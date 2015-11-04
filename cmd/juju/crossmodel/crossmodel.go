// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The crossmodel command provides an interface that allows to
// manipulate and inspect cross model relation.
package crossmodel

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.crossmodel")

// CrossModelCommandBase is a base structure to get cross model managing client.
type CrossModelCommandBase struct {
	envcmd.EnvCommandBase
}

// NewCrossModelAPI returns a cross model api for the root api endpoint
// that the environment command returns.
func (c *CrossModelCommandBase) NewCrossModelAPI() (*crossmodel.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return crossmodel.NewClient(root), nil
}

// SAASEndpoint defines the serialization behaviour of SAAS endpoint details.
type SAASEndpoint struct {
	// Service has service name.
	Service string `yaml:"service" json:"service"`

	// Endpoints list of SAAS endpoints.
	Endpoints []string `yaml:"endpoints" json:"endpoints"`

	// Desc is the user entered description.
	Desc string `yaml:"desc,omitempty" json:"desc,omitempty"`
}

// formatEndpoints takes any number of api-formatted SAAS endpoints and
// creates a collection of ui-formatted endpoints.
func formatEndpoints(apiEndpoints ...params.SAASDetailsResult) ([]SAASEndpoint, error) {
	if len(apiEndpoints) == 0 {
		return nil, nil
	}
	output := make([]SAASEndpoint, len(apiEndpoints))
	for i, one := range apiEndpoints {
		serviceName, err := getServiceNameFromTag(one.Service)
		if err != nil {
			return nil, errors.Trace(err)
		}
		output[i].Service = serviceName
		output[i].Endpoints = one.Endpoints
		if one.Description != "" {
			output[i].Desc = one.Description
		}
	}
	return output, nil
}

func getServiceNameFromTag(serviceTag string) (string, error) {
	tag, err := names.ParseServiceTag(serviceTag)
	if err != nil {
		return "", errors.Annotatef(err, "could not parse service tag %q", serviceTag)
	}
	return tag.Name, nil
}
