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

// RemoteEndpoint defines the serialization behaviour of remote endpoints.
type RemoteEndpoint struct {
	// Interface is relation interface.
	Interface string `yaml:"interface" json:"interface"`

	// Role is relation role.
	Role string `yaml:"role" json:"role"`
}

// RemoteService defines the serialization behaviour of remote service.
type RemoteService struct {
	// Endpoints list of offered service endpoints.
	Endpoints map[string]RemoteEndpoint `yaml:"endpoints" json:"endpoints"`

	// Description is the user entered description.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// convertRemoteServices takes any number of api-formatted remote services and
// creates a collection of ui-formatted services.
func convertRemoteServices(services ...params.RemoteServiceInfo) (map[string]RemoteService, error) {
	if len(services) == 0 {
		return nil, nil
	}
	output := make(map[string]RemoteService, len(services))
	for _, one := range services {
		serviceName, err := getServiceNameFromTag(one.Service)
		if err != nil {
			return nil, errors.Trace(err)
		}
		service := RemoteService{Endpoints: convertRemoteEndpoints(one.Endpoints...)}
		if one.Description != "" {
			service.Description = one.Description
		}
		output[serviceName] = service
	}
	return output, nil
}

// convertRemoteEndpoints takes any number of api-formatted remote services' endpoints and
// creates a collection of ui-formatted endpoints.
func convertRemoteEndpoints(apiEndpoints ...params.RemoteEndpoint) map[string]RemoteEndpoint {
	if len(apiEndpoints) == 0 {
		return nil
	}
	output := make(map[string]RemoteEndpoint, len(apiEndpoints))
	for _, one := range apiEndpoints {
		output[one.Name] = RemoteEndpoint{one.Interface, one.Role}
	}
	return output
}

func getServiceNameFromTag(serviceTag string) (string, error) {
	tag, err := names.ParseServiceTag(serviceTag)
	if err != nil {
		return "", errors.Annotatef(err, "could not parse service tag %q", serviceTag)
	}
	return tag.Name, nil
}
