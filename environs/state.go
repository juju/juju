// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/instance"
)

// StateFile is the name of the file where the provider's state is stored.
const StateFile = "provider-state"

// BootstrapState is the state information that is stored in StateFile.
//
// Individual providers may define their own state structures instead of
// this one, and use their own code for loading and saving those, but this is
// the definition that most practically useful providers share unchanged.
type BootstrapState struct {
	// StateInstances are the state servers.
	StateInstances []instance.Id `yaml:"state-instances"`
}

// SaveState writes the given state to the given storage.
func SaveState(storage StorageWriter, state *BootstrapState) error {
	data, err := goyaml.Marshal(state)
	if err != nil {
		return err
	}
	return storage.Put(StateFile, bytes.NewBuffer(data), int64(len(data)))
}

// LoadState reads state from the given storage.
func LoadState(storage StorageReader) (*BootstrapState, error) {
	r, err := storage.Get(StateFile)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("error reading %q: %v", StateFile, err)
	}
	var state BootstrapState
	err = goyaml.Unmarshal(data, &state)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling %q: %v", StateFile, err)
	}
	return &state, nil
}
