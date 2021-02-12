// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"github.com/juju/errors"
	"github.com/juju/replicaset"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

// This file holds code that translates from State
// to the interface expected internally by the worker.

type StateShim struct {
	*state.State
}

func (s StateShim) ControllerNode(id string) (ControllerNode, error) {
	return s.State.ControllerNode(id)
}

func (s StateShim) ControllerHost(id string) (ControllerHost, error) {
	// For IAAS models, controllers are machines.
	// For CAAS models, access to the controller is via a k8s service.
	model, err := s.State.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if model.Type() == state.ModelTypeIAAS {
		return s.State.Machine(id)
	}

	cloudService, err := s.State.CloudService(model.ControllerUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &cloudServiceShim{cloudService}, nil
}

func (s StateShim) RemoveControllerReference(c ControllerNode) error {
	return s.State.RemoveControllerReference(c)
}

func (s StateShim) Space(name string) (Space, error) {
	return s.State.SpaceByName(name)
}

// cloudServiceShim stubs out functionality not yet
// supported by the k8s service abstraction.
// We don't yet support HA on k8s.
type cloudServiceShim struct {
	*state.CloudService
}

func (*cloudServiceShim) Life() state.Life {
	// We don't manage the life of a cloud service entity.
	return state.Alive
}

func (*cloudServiceShim) Status() (status.StatusInfo, error) {
	// We don't record the status of a cloud service entity.
	return status.StatusInfo{
		Status: status.Active,
	}, nil
}

func (*cloudServiceShim) SetStatus(status.StatusInfo) error {
	// We don't record the status of a cloud service entity.
	return nil
}

// MongoSessionShim wraps a *mgo.Session to conform to the
// MongoSession interface.
type MongoSessionShim struct {
	*mgo.Session
}

func (s MongoSessionShim) CurrentStatus() (*replicaset.Status, error) {
	return replicaset.CurrentStatus(s.Session)
}

func (s MongoSessionShim) CurrentMembers() ([]replicaset.Member, error) {
	return replicaset.CurrentMembers(s.Session)
}

func (s MongoSessionShim) Set(members []replicaset.Member) error {
	return replicaset.Set(s.Session, members)
}

func (s MongoSessionShim) StepDownPrimary() error {
	return replicaset.StepDownPrimary(s.Session)
}

func (s MongoSessionShim) Refresh() {
	s.Session.Refresh()
}
