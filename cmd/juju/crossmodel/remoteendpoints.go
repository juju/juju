// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/charm/v7"

	"github.com/juju/juju/api/applicationoffers"
	"github.com/juju/juju/cmd/modelcmd"
)

// RemoteEndpointsCommandBase is a base for various cross model commands.
type RemoteEndpointsCommandBase struct {
	modelcmd.ControllerCommandBase
}

// NewRemoteEndpointsAPI returns a remote endpoints api for the root api endpoint
// that the command returns.
func (c *RemoteEndpointsCommandBase) NewRemoteEndpointsAPI(controllerName string) (*applicationoffers.Client, error) {
	root, err := c.CommandBase.NewAPIRoot(c.ClientStore(), controllerName, "")
	if err != nil {
		return nil, err
	}
	return applicationoffers.NewClient(root), nil
}

// RemoteEndpoint defines the serialization behaviour of remote endpoints.
// This is used in map-style yaml output where remote endpoint name is the key.
type RemoteEndpoint struct {
	// Name is the endpoint name.
	Name string `yaml:"-" json:"-"`

	// Interface is relation interface.
	Interface string `yaml:"interface" json:"interface"`

	// Role is relation role.
	Role string `yaml:"role" json:"role"`
}

// convertRemoteEndpoints takes any number of api-formatted remote applications' endpoints and
// creates a collection of ui-formatted endpoints.
func convertRemoteEndpoints(apiEndpoints ...charm.Relation) map[string]RemoteEndpoint {
	if len(apiEndpoints) == 0 {
		return nil
	}
	output := make(map[string]RemoteEndpoint, len(apiEndpoints))
	for _, one := range apiEndpoints {
		output[one.Name] = RemoteEndpoint{one.Name, one.Interface, string(one.Role)}
	}
	return output
}
