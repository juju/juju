// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/state"
)

// The interfaces below are used to create mocks for testing.

type Backend interface {
	Model() (*state.Model, error)
	Application(name string) (Application, error)
	MongoVersion() (string, error)
	ControllerModelUUID() string
	AllModelUUIDs() ([]string, error)
	ModelExists(uuid string) (bool, error)
}

type Application interface {
	Name() string
	Relations() ([]Relation, error)
	CharmConfig() (charm.Settings, error)
}

type Relation interface {
	Endpoint(applicationname string) (relation.Endpoint, error)
	RelatedEndpoints(applicationname string) ([]relation.Endpoint, error)
	ApplicationSettings(appName string) (map[string]interface{}, error)
	ModelUUID() string
}

type stateShim struct {
	*state.State
}

func (s stateShim) Application(name string) (Application, error) {
	app, err := s.State.Application(name)
	return applicationShim{app}, err
}

type applicationShim struct {
	*state.Application
}

func (a applicationShim) Relations() ([]Relation, error) {
	rels, err := a.Application.Relations()
	if err != nil {
		return nil, err
	}
	result := make([]Relation, len(rels))
	for i, r := range rels {
		result[i] = r
	}
	return result, nil
}
