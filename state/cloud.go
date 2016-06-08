// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"github.com/juju/errors"
)

// cloudGlobalKey returns the global database key for the specified cloud.
func cloudGlobalKey(name string) string {
	return fmt.Sprintf("%s#%s#cloud", modelGlobalKey, name)
}

// ControllerConfig returns the config values shared across models.
func (st *State) SharedCloudConfig() (map[string]interface{}, error) {
	model, err := st.Model()
	if err != nil {
		return nil, err
	}
	settings, err := readSettings(st, cloudSettingsC, cloudGlobalKey(model.Cloud()))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return settings.Map(), nil
}
