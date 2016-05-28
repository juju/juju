// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/names"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// This file contains untested shims to let us wrap state in a sensible
// interface and avoid writing tests that depend on mongodb. If you were
// to change any part of it so that it were no longer *obviously* and
// *trivially* correct, you would be Doing It Wrong.

func init() {
	common.RegisterStandardFacade("Backups", 1, newAPI)
}

// Machine represents a state.Machine, it is used to break dependency
// to state.
type Machine interface {
	Series() string
	PublicAddress() (network.Address, error)
	PrivateAddress() (network.Address, error)
	InstanceId() (instance.Id, error)
	Tag() names.Tag
}

type stateShim struct {
	st *state.State
}

func (s *stateShim) IsController() bool {
	return s.st.IsController()
}

func (s *stateShim) MongoSession() *mgo.Session {
	return s.st.MongoSession()
}

func (s *stateShim) MongoConnectionInfo() *mongo.MongoInfo {
	return s.st.MongoConnectionInfo()
}

func (s *stateShim) ModelTag() names.ModelTag {
	return s.st.ModelTag()
}

func (s *stateShim) Machine(id string) (Machine, error) {
	return s.st.Machine(id)
}

func (s *stateShim) ModelConfig() (*config.Config, error) {
	return s.st.ModelConfig()
}

func (s *stateShim) StateServingInfo() (state.StateServingInfo, error) {
	return s.st.StateServingInfo()
}

func (s *stateShim) RestoreInfo() *state.RestoreInfo {
	return s.st.RestoreInfo()
}

func newAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*API, error) {
	return NewAPI(&stateShim{st}, resources, authorizer)
}
