// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	jujucontroller "github.com/juju/juju/controller"
)

const (
	// ControllerSettingsGlobalKey is the key for the controller and its settings.
	ControllerSettingsGlobalKey = "controllerSettings"

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
//
// This type is primarily used in the state.Initialize function, and
// in the yet to be hooked up controller worker.
type Controller struct {
	pool     *StatePool
	ownsPool bool
}

// NewController returns a controller object that doesn't own
// the state pool it has been given. This is for convenience
// at this time to get access to controller methods.
func NewController(pool *StatePool) *Controller {
	return &Controller{pool: pool}
}

// StatePool provides access to the state pool of the controller.
func (ctlr *Controller) StatePool() *StatePool {
	return ctlr.pool
}

// SystemState returns the State object for the controller model.
func (ctlr *Controller) SystemState() *State {
	return ctlr.pool.SystemState()
}

// Close the connection to the database.
func (ctlr *Controller) Close() error {
	if ctlr.ownsPool {
		ctlr.pool.Close()
	}
	return nil
}

// GetState returns a new State instance for the specified model. The
// connection uses the same credentials and policy as the Controller.
func (ctlr *Controller) GetState(modelTag names.ModelTag) (*PooledState, error) {
	return ctlr.pool.Get(modelTag.Id())
}

// Ping probes the Controllers's database connection to ensure that it
// is still alive.
func (ctlr *Controller) Ping() error {
	if ctlr.pool.SystemState() == nil {
		return errors.New("pool is closed")
	}
	return ctlr.pool.SystemState().Ping()
}

// ControllerConfig returns the config values for the controller.
func (st *State) ControllerConfig() (jujucontroller.Config, error) {
	settings, err := readSettings(st.db(), controllersC, ControllerSettingsGlobalKey)
	if err != nil {
		return nil, errors.Annotatef(err, "controller %q", st.ControllerUUID())
	}
	return settings.Map(), nil
}

// UpdateControllerConfig allows changing some of the configuration
// for the controller. Changes passed in updateAttrs will be applied
// to the current config, and keys in removeAttrs will be unset (and
// so revert to their defaults). Only a subset of keys can be changed
// after bootstrapping.
func (st *State) UpdateControllerConfig(updateAttrs map[string]interface{}, removeAttrs []string) error {
	if err := st.checkValidControllerConfig(updateAttrs, removeAttrs); err != nil {
		return errors.Trace(err)
	}

	settings, err := readSettings(st.db(), controllersC, ControllerSettingsGlobalKey)
	if err != nil {
		return errors.Annotatef(err, "controller %q", st.ControllerUUID())
	}
	for _, r := range removeAttrs {
		settings.Delete(r)
	}
	settings.Update(updateAttrs)

	// Ensure the resulting config is still valid.
	newValues := settings.Map()
	_, err = jujucontroller.NewConfig(
		newValues[jujucontroller.ControllerUUIDKey].(string),
		newValues[jujucontroller.CACertKey].(string),
		newValues,
	)
	if err != nil {
		return errors.Trace(err)
	}

	_, ops := settings.settingsUpdateOps()
	return errors.Trace(settings.write(ops))
}

func (st *State) checkValidControllerConfig(updateAttrs map[string]interface{}, removeAttrs []string) error {
	for k := range updateAttrs {
		if err := checkUpdateControllerConfig(k); err != nil {
			return errors.Trace(err)
		}

		if k == jujucontroller.JujuHASpace || k == jujucontroller.JujuManagementSpace {
			cVal := updateAttrs[k].(string)
			if err := st.checkSpaceIsAvailableToAllControllers(cVal); err != nil {
				return errors.Annotatef(err, "invalid config %q=%q", k, cVal)
			}
		}
	}
	for _, r := range removeAttrs {
		if err := checkUpdateControllerConfig(r); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func checkUpdateControllerConfig(name string) error {
	if !jujucontroller.ControllerOnlyAttribute(name) {
		return errors.Errorf("unknown controller config setting %q", name)
	}
	if !jujucontroller.AllowedUpdateConfigAttributes.Contains(name) {
		return errors.Errorf("can't change %q after bootstrap", name)
	}
	return nil
}

// checkSpaceIsAvailableToAllControllers checks if each controller machine has
// at least one address in the input space. If not, an error is returned.
func (st *State) checkSpaceIsAvailableToAllControllers(spaceName string) error {
	controllerIds, err := st.ControllerIds()
	if err != nil {
		return errors.Annotate(err, "cannot get controller info")
	}

	space, err := st.SpaceByName(spaceName)
	if err != nil {
		return errors.Trace(err)
	}
	netSpace, err := space.NetworkSpace()
	if err != nil {
		return errors.Annotate(err, "getting network space")
	}

	var missing []string
	for _, id := range controllerIds {
		m, err := st.Machine(id)
		if err != nil {
			return errors.Annotate(err, "cannot get machine")
		}
		if _, ok := m.Addresses().InSpaces(netSpace); !ok {
			missing = append(missing, id)
		}
	}

	if len(missing) > 0 {
		return errors.Errorf("machines with no addresses in this space: %s", strings.Join(missing, ", "))
	}
	return nil
}

type controllersDoc struct {
	Id            string   `bson:"_id"`
	CloudName     string   `bson:"cloud"`
	ModelUUID     string   `bson:"model-uuid"`
	ControllerIds []string `bson:"controller-ids"`
}

// ControllerInfo holds information about currently
// configured controller machines.
type ControllerInfo struct {
	// CloudName is the name of the cloud to which this controller is deployed.
	CloudName string

	// ModelTag identifies the initial model. Only the initial
	// model is able to have machines that manage state. The initial
	// model is the model that is created when bootstrapping.
	ModelTag names.ModelTag

	// ControllerIds holds the ids of all the controller nodes.
	// It's main purpose is to allow assertions tha the set of
	// controllers hasn't changed when adding/removing controller nodes.
	ControllerIds []string
}

// ControllerInfo returns information about
// the currently configured controller machines.
func (st *State) ControllerInfo() (*ControllerInfo, error) {
	session := st.session.Copy()
	defer session.Close()
	return readRawControllerInfo(session)
}

// readRawControllerInfo reads ControllerInfo direct from the supplied session,
// falling back to the bootstrap model document to extract the UUID when
// required.
func readRawControllerInfo(session *mgo.Session) (*ControllerInfo, error) {
	db := session.DB(jujuDB)
	controllers := db.C(controllersC)

	var doc controllersDoc
	err := controllers.Find(bson.D{{"_id", modelGlobalKey}}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("controllers document")
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get controllers document")
	}
	return &ControllerInfo{
		CloudName:     doc.CloudName,
		ModelTag:      names.NewModelTag(doc.ModelUUID),
		ControllerIds: doc.ControllerIds,
	}, nil
}

const stateServingInfoKey = "stateServingInfo"

type stateServingInfo struct {
	APIPort      int    `bson:"apiport"`
	StatePort    int    `bson:"stateport"`
	Cert         string `bson:"cert"`
	PrivateKey   string `bson:"privatekey"`
	CAPrivateKey string `bson:"caprivatekey"`
	// this will be passed as the KeyFile argument to MongoDB
	SharedSecret   string `bson:"sharedsecret"`
	SystemIdentity string `bson:"systemidentity"`
}

// StateServingInfo returns information for running a controller machine
func (st *State) StateServingInfo() (jujucontroller.StateServingInfo, error) {
	controllers, closer := st.db().GetCollection(controllersC)
	defer closer()

	var info stateServingInfo
	err := controllers.Find(bson.D{{"_id", stateServingInfoKey}}).One(&info)
	if err != nil {
		return jujucontroller.StateServingInfo{}, errors.Trace(err)
	}
	if info.StatePort == 0 {
		return jujucontroller.StateServingInfo{}, errors.NotFoundf("state serving info")
	}
	return jujucontroller.StateServingInfo{
		APIPort:        info.APIPort,
		StatePort:      info.StatePort,
		Cert:           info.Cert,
		PrivateKey:     info.PrivateKey,
		CAPrivateKey:   info.CAPrivateKey,
		SharedSecret:   info.SharedSecret,
		SystemIdentity: info.SystemIdentity,
	}, nil
}

// SetStateServingInfo stores information needed for running a controller
func (st *State) SetStateServingInfo(info jujucontroller.StateServingInfo) error {
	if info.StatePort == 0 || info.APIPort == 0 ||
		info.Cert == "" || info.PrivateKey == "" {
		return errors.Errorf("incomplete state serving info set in state")
	}
	if info.CAPrivateKey == "" {
		// No CA certificate key means we can't generate new controller
		// certificates when needed to add to the certificate SANs.
		// Older Juju deployments discard the key because no one realised
		// the certificate was flawed, so at best we can log a warning
		// until an upgrade process is written.
		logger.Warningf("state serving info has no CA certificate key")
	}
	ops := []txn.Op{{
		C:  controllersC,
		Id: stateServingInfoKey,
		Update: bson.D{{"$set", stateServingInfo{
			APIPort:        info.APIPort,
			StatePort:      info.StatePort,
			Cert:           info.Cert,
			PrivateKey:     info.PrivateKey,
			CAPrivateKey:   info.CAPrivateKey,
			SharedSecret:   info.SharedSecret,
			SystemIdentity: info.SystemIdentity,
		}}},
	}}
	if err := st.db().RunTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot set state serving info")
	}
	return nil
}
