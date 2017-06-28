// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	names "gopkg.in/juju/names.v2"
	mgo "gopkg.in/mgo.v2"

	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/mongo"
)

const (
	// controllerSettingsGlobalKey is the key for the controller and its settings.
	controllerSettingsGlobalKey = "controllerSettings"

	// controllerGlobalKey is the key for controller.
	controllerGlobalKey = "c"
)

// controllerKey will return the key for a given controller using the
// controller uuid and the controllerGlobalKey.
func controllerKey(controllerUUID string) string {
	return fmt.Sprintf("%s#%s", controllerGlobalKey, controllerUUID)
}

// Controller encapsulates state for the Juju controller as a whole,
// as opposed to model specific functionality.
type Controller struct {
	clock                  clock.Clock
	controllerModelTag     names.ModelTag
	controllerTag          names.ControllerTag
	mongoInfo              *mongo.MongoInfo
	session                *mgo.Session
	policy                 Policy
	newPolicy              NewPolicyFunc
	runTransactionObserver RunTransactionObserverFunc
}

// Close the connection to the database.
func (ctlr *Controller) Close() error {
	ctlr.session.Close()
	return nil
}

// NewState returns a new State instance for the specified model. The
// connection uses the same credentials and policy as the Controller.
func (ctlr *Controller) NewState(modelTag names.ModelTag) (*State, error) {
	session := ctlr.session.Copy()
	st, err := newState(
		modelTag,
		ctlr.controllerModelTag,
		session,
		ctlr.mongoInfo,
		ctlr.newPolicy,
		ctlr.clock,
		ctlr.runTransactionObserver,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := st.start(ctlr.controllerTag); err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// ControllerConfig returns the config values for the controller.
func (st *State) ControllerConfig() (jujucontroller.Config, error) {
	settings, err := readSettings(st, controllersC, controllerSettingsGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return settings.Map(), nil
}
