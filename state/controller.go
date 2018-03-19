// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"

	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/network"
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
//
// TODO(menn0) - this is currently unused, pending further refactoring
// of State.
type Controller struct {
	clock                  clock.Clock
	controllerModelTag     names.ModelTag
	controllerTag          names.ControllerTag
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
		ctlr.newPolicy,
		ctlr.clock,
		ctlr.runTransactionObserver,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := st.start(ctlr.controllerTag, nil); err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// Ping probes the Controllers's database connection to ensure that it
// is still alive.
func (ctlr *Controller) Ping() error {
	return ctlr.session.Ping()
}

// ControllerConfig returns the config values for the controller.
func (st *State) ControllerConfig() (jujucontroller.Config, error) {
	settings, err := readSettings(st.db(), controllersC, controllerSettingsGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
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

	settings, err := readSettings(st.db(), controllersC, controllerSettingsGlobalKey)
	if err != nil {
		return errors.Trace(err)
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
			if err := st.checkSpaceIsAvailableToAllControllers(updateAttrs[k].(string)); err != nil {
				return errors.Annotatef(err, "invalid config value for %q", k)
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
func (st *State) checkSpaceIsAvailableToAllControllers(configSpace string) error {
	info, err := st.ControllerInfo()
	if err != nil {
		return errors.Annotate(err, "cannot get controller info")
	}

	var missing []string
	spaceName := network.SpaceName(configSpace)
	for _, id := range info.MachineIds {
		m, err := st.Machine(id)
		if err != nil {
			return errors.Annotate(err, "cannot get machine")
		}
		if _, ok := network.SelectAddressesBySpaceNames(m.Addresses(), spaceName); !ok {
			missing = append(missing, id)
		}
	}

	if len(missing) > 0 {
		mStr := strings.Trim(fmt.Sprintf("%q", missing), "[]")
		return errors.Errorf("machines with no addresses in space %q: %s", configSpace, mStr)
	}
	return nil
}
