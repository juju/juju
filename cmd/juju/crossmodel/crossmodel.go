// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The crossmodel command provides an interface that allows to
// manipulate and inspect cross model relations.
package crossmodel

import (
	"github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

// CrossModelCommandBase is a base structure to get cross model managing client.
type CrossModelCommandBase struct {
	modelcmd.ModelCommandBase
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
// This is used in map-style yaml output where remote endpoint name is the key.
type RemoteEndpoint struct {
	// Interface is relation interface.
	Interface string `yaml:"interface" json:"interface"`

	// Role is relation role.
	Role string `yaml:"role" json:"role"`
}

// convertRemoteEndpoints takes any number of api-formatted remote applications' endpoints and
// creates a collection of ui-formatted endpoints.
func convertRemoteEndpoints(apiEndpoints ...params.RemoteEndpoint) map[string]RemoteEndpoint {
	if len(apiEndpoints) == 0 {
		return nil
	}
	output := make(map[string]RemoteEndpoint, len(apiEndpoints))
	for _, one := range apiEndpoints {
		output[one.Name] = RemoteEndpoint{one.Interface, string(one.Role)}
	}
	return output
}
