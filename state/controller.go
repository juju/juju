// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

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
	settings, err := readSettings(st.db(), controllersC, controllerSettingsGlobalKey)
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

	settings, err := readSettings(st.db(), controllersC, controllerSettingsGlobalKey)
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
		return errors.Errorf("machines with no addresses in this space: %s", strings.Join(missing, ", "))
	}
	return nil
}
