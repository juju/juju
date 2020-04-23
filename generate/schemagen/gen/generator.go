// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package gen

import (
	"reflect"

	"github.com/juju/errors"
	jsonschema "github.com/juju/jsonschema-gen"
	"github.com/juju/rpcreflect"

	"github.com/juju/juju/apiserver/facade"
)

//go:generate go run github.com/golang/mock/mockgen -package gen -destination describeapi_mock.go github.com/juju/juju/generate/schemagen/gen APIServer,Registry
type APIServer interface {
	AllFacades() Registry
}

type Registry interface {
	List() []facade.Description
	GetType(name string, version int) (reflect.Type, error)
}

// Generate a FacadeSchema from the APIServer
func Generate(client APIServer) ([]FacadeSchema, error) {
	registry := client.AllFacades()
	facades := registry.List()
	result := make([]FacadeSchema, len(facades))
	for i, facade := range facades {
		// select the latest version from the facade list
		version := facade.Versions[len(facade.Versions)-1]

		result[i].Name = facade.Name
		result[i].Version = version

		kind, err := registry.GetType(facade.Name, version)
		if err != nil {
			return nil, errors.Annotatef(err, "getting type for facade %s at version %d", facade.Name, version)
		}
		objType := rpcreflect.ObjTypeOf(kind)
		result[i].Schema = jsonschema.ReflectFromObjType(objType)
	}
	return result, nil
}

type FacadeSchema struct {
	Name    string
	Version int
	Schema  *jsonschema.Schema
}
