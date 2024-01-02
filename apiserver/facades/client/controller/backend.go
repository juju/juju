// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/charm/v12"
	"github.com/juju/names/v5"

	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
)

// The interfaces below are used to create mocks for testing.

type ControllerAccess interface {
	ControllerTag() names.ControllerTag
	AddControllerUser(spec state.UserAccessSpec) (permission.UserAccess, error)
	UserAccess(subject names.UserTag, target names.Tag) (permission.UserAccess, error)
	ControllerInfo() (*state.ControllerInfo, error)
	CreateCloudAccess(cloud string, user names.UserTag, access permission.Access) error
	GetCloudAccess(cloud string, user names.UserTag) (permission.Access, error)
	RemoveCloudAccess(cloud string, user names.UserTag) error
	UserPermission(subject names.UserTag, target names.Tag) (permission.Access, error)
	RemoveUserAccess(subject names.UserTag, target names.Tag) error
	SetUserAccess(subject names.UserTag, target names.Tag, access permission.Access) (permission.UserAccess, error)
}

type Backend interface {
	ControllerAccess
	Model() (*state.Model, error)
	Application(name string) (Application, error)
	MongoVersion() (string, error)
	ControllerModelUUID() string
	AllModelUUIDs() ([]string, error)
	AllBlocksForController() ([]state.Block, error)
	RemoveAllBlocksForController() error
	ModelExists(uuid string) (bool, error)
	ControllerConfig() (jujucontroller.Config, error)
	UpdateControllerConfig(updateAttrs map[string]interface{}, removeAttrs []string) error
}

type Application interface {
	Name() string
	Relations() ([]Relation, error)
	CharmConfig(branchName string) (charm.Settings, error)
}

type Relation interface {
	Endpoint(applicationname string) (state.Endpoint, error)
	RelatedEndpoints(applicationname string) ([]state.Endpoint, error)
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
