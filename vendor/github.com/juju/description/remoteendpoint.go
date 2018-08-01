// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
)

// RemoteEndpoint represents a connection point that can be related to
// another application.
type RemoteEndpoint interface {
	Name() string
	Role() string
	Interface() string
}

type remoteEndpoints struct {
	Version   int               `yaml:"version"`
	Endpoints []*remoteEndpoint `yaml:"endpoints"`
}

type remoteEndpoint struct {
	Name_      string `yaml:"name"`
	Role_      string `yaml:"role"`
	Interface_ string `yaml:"interface"`
}

// RemoteEndpointArgs is an argument struct used to add a remote
// endpoint to a remote application.
type RemoteEndpointArgs struct {
	Name      string
	Role      string
	Interface string
}

func newRemoteEndpoint(args RemoteEndpointArgs) *remoteEndpoint {
	return &remoteEndpoint{
		Name_:      args.Name,
		Role_:      args.Role,
		Interface_: args.Interface,
	}
}

// Name implements RemoteEndpoint.
func (e *remoteEndpoint) Name() string {
	return e.Name_
}

// Role implements RemoteEndpoint.
func (e *remoteEndpoint) Role() string {
	return e.Role_
}

// Interface implements RemoteEndpoint.
func (e *remoteEndpoint) Interface() string {
	return e.Interface_
}

func importRemoteEndpoints(sourceMap map[string]interface{}) ([]*remoteEndpoint, error) {
	checker := versionedChecker("endpoints")
	coerced, err := checker.Coerce(sourceMap, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "remote endpoints version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := remoteEndpointDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["endpoints"].([]interface{})
	return importRemoteEndpointList(sourceList, importFunc)
}

func importRemoteEndpointList(sourceList []interface{}, importFunc remoteEndpointDeserializationFunc) ([]*remoteEndpoint, error) {
	var result []*remoteEndpoint
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for remote endpoint %d, %T", i, value)
		}
		device, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "remote endpoint %d", i)
		}
		result = append(result, device)
	}
	return result, nil
}

type remoteEndpointDeserializationFunc func(map[string]interface{}) (*remoteEndpoint, error)

var remoteEndpointDeserializationFuncs = map[int]remoteEndpointDeserializationFunc{
	1: importRemoteEndpointV1,
}

func importRemoteEndpointV1(source map[string]interface{}) (*remoteEndpoint, error) {
	fields := schema.Fields{
		"name":      schema.String(),
		"role":      schema.String(),
		"interface": schema.String(),
	}

	checker := schema.FieldMap(fields, nil)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "remote endpoint v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result := &remoteEndpoint{
		Name_:      valid["name"].(string),
		Role_:      valid["role"].(string),
		Interface_: valid["interface"].(string),
	}

	return result, nil
}
