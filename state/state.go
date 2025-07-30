// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/semversion"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.state")

// State represents the state of an model
// managed by juju.
type State struct {
	stateClock         clock.Clock
	modelTag           names.ModelTag
	controllerModelTag names.ModelTag
	controllerTag      names.ControllerTag
	policy             Policy
	newPolicy          NewPolicyFunc
	maxTxnAttempts     int
}

// IsController returns true if this state instance has the bootstrap
// model UUID.
func (st *State) IsController() bool {
	return st.modelTag == st.controllerModelTag
}

// ControllerUUID returns the UUID for the controller
// of this state instance.
func (st *State) ControllerUUID() string {
	return st.controllerTag.Id()
}

// ControllerTag returns the tag form of the ControllerUUID.
func (st *State) ControllerTag() names.ControllerTag {
	return st.controllerTag
}

// ControllerTimestamp returns the current timestamp of the backend
// controller.
func (st *State) ControllerTimestamp() (*time.Time, error) {
	now := time.Now()
	return &now, nil
}

// ControllerModelUUID returns the UUID of the model that was
// bootstrapped.  This is the only model that can have controller
// machines.  The owner of this model is also considered "special", in
// that they are the only user that is able to create other users
// (until we have more fine grained permissions), and they cannot be
// disabled.
func (st *State) ControllerModelUUID() string {
	return st.controllerModelTag.Id()
}

// RemoveDyingModel sets current model to dead then removes all documents from
// multi-model collections.
func (st *State) RemoveDyingModel() error {
	return nil
}

// ModelUUID returns the model UUID for the model
// controlled by this state instance.
func (st *State) ModelUUID() string {
	return st.modelTag.Id()
}

// EnsureModelRemoved returns an error if any multi-model
// documents for this model are found. It is intended only to be used in
// tests and exported so it can be used in the tests of other packages.
func (st *State) EnsureModelRemoved() error {
	return nil
}

// Ping probes the state's database connection to ensure
// that it is still alive.
func (st *State) Ping() error {
	return nil
}

// MongoVersion return the string repre
func (st *State) MongoVersion() (string, error) {
	return "-4.4", nil
}

// Upgrader is an interface that can be used to check if an upgrade is in
// progress.
type Upgrader interface {
	IsUpgrading() (bool, error)
}

// SetModelAgentVersion changes the agent version for the model to the
// given version, only if the model is in a stable state (all agents are
// running the current version). If this is a hosted model, newVersion
// cannot be higher than the controller version.
func (st *State) SetModelAgentVersion(newVersion semversion.Number, stream *string, ignoreAgentVersions bool, upgrader Upgrader) (err error) {
	return nil
}

// Report conforms to the Dependency Engine Report() interface, giving an opportunity to introspect
// what is going on at runtime.
func (st *State) Report() map[string]interface{} {
	return nil
}

// TagFromDocID tries attempts to extract an entity-identifying tag from a
// Mongo document ID.
// For example "c9741ea1-0c2a-444d-82f5-787583a48557:a#mediawiki" would yield
// an application tag for "mediawiki"
func TagFromDocID(docID string) names.Tag {
	return nil
}
