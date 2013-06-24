// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/instance"
)

const stateFile = "provider-state"

// Persistent environment state.  An environment needs to know what resources
// in the Azure cloud it manages.
type bootstrapState struct {
	StateInstances []instance.Id `yaml:"state-instances"`
}

// saveState writes the environment's state to the provider-state file stored
// in the environment's storage.
func (env *azureEnviron) saveState(state *bootstrapState) error {
	data, err := goyaml.Marshal(state)
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(data)
	return env.Storage().Put(stateFile, buf, int64(len(data)))
}

// loadState reads the environment's state from storage.
func (env *azureEnviron) loadState() (*bootstrapState, error) {
	r, err := env.Storage().Get(stateFile)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("error reading %q: %v", stateFile, err)
	}
	var state bootstrapState
	err = goyaml.Unmarshal(data, &state)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling %q: %v", stateFile, err)
	}
	return &state, nil
}
