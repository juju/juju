// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"

	"launchpad.net/goyaml"

	"io"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"net/http"
)

// StateFile is the name of the file where the provider's state is stored.
const StateFile = "provider-state"

// BootstrapState is the state information that is stored in StateFile.
//
// Individual providers may define their own state structures instead of
// this one, and use their own code for loading and saving those, but this is
// the definition that most practically useful providers share unchanged.

type InstanceInfo struct {
	Id              instance.Id
	Characteristics instance.HardwareCharacteristics
}

type BootstrapState struct {
	// StateInstances are the state servers.
	StateInstances []InstanceInfo `yaml:"state-instances"`
}

// SaveState writes the given state to the given storage.
func SaveState(storage StorageWriter, state *BootstrapState) error {
	data, err := goyaml.Marshal(state)
	if err != nil {
		return err
	}
	return storage.Put(StateFile, bytes.NewBuffer(data), int64(len(data)))
}

// LoadStateFromURL reads state from the given URL.
func LoadStateFromURL(url string) (*BootstrapState, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	return loadState(resp.Body)
}

// LoadState reads state from the given storage.
func LoadState(storage StorageReader) (*BootstrapState, error) {
	r, err := storage.Get(StateFile)
	if err != nil {
		return nil, err
	}
	return loadState(r)
}

func loadState(r io.ReadCloser) (*BootstrapState, error) {
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

// getDNSNames queries and returns the DNS names for the given instances,
// ignoring nil instances or ones without DNS names.
func getDNSNames(instances []instance.Instance) []string {
	names := make([]string, 0)
	for _, inst := range instances {
		if inst != nil {
			name, err := inst.DNSName()
			// If that fails, just keep looking.
			if err == nil && name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// composeAddresses suffixes each of a slice of hostnames with a given port
// number.
func composeAddresses(hostnames []string, port int) []string {
	addresses := make([]string, len(hostnames))
	for index, hostname := range hostnames {
		addresses[index] = fmt.Sprintf("%s:%d", hostname, port)
	}
	return addresses
}

// composeStateInfo puts together the state.Info and api.Info for the given
// config, with the given state-server host names.
// The given config absolutely must have a CACert.
func getStateInfo(config *config.Config, hostnames []string) (*state.Info, *api.Info) {
	cert, hasCert := config.CACert()
	if !hasCert {
		panic(errors.New("getStateInfo: config has no CACert"))
	}
	return &state.Info{
			Addrs:  composeAddresses(hostnames, config.StatePort()),
			CACert: cert,
		}, &api.Info{
			Addrs:  composeAddresses(hostnames, config.APIPort()),
			CACert: cert,
		}
}

// StateInfo is a reusable implementation of Environ.StateInfo, available to
// providers that also use the other functionality from this file.
func StateInfo(env Environ) (*state.Info, *api.Info, error) {
	st, err := LoadState(env.Storage())
	if err != nil {
		return nil, nil, err
	}
	config := env.Config()
	if _, hasCert := config.CACert(); !hasCert {
		return nil, nil, fmt.Errorf("no CA certificate in environment configuration")
	}
	// Wait for the DNS names of any of the instances
	// to become available.
	log.Debugf("waiting for DNS name(s) of state server instances %v", st.StateInstances)
	var hostnames []string
	var ids []instance.Id
	for _, stateInst := range st.StateInstances {
		ids = append(ids, stateInst.Id)
	}
	for a := LongAttempt.Start(); len(hostnames) == 0 && a.Next(); {
		insts, err := env.Instances(ids)
		if err != nil && err != ErrPartialInstances {
			log.Debugf("error getting state instances: %v", err.Error())
			return nil, nil, err
		}
		hostnames = getDNSNames(insts)
	}

	if len(hostnames) == 0 {
		return nil, nil, fmt.Errorf("timed out waiting for mgo address from %v", st.StateInstances)
	}

	stateInfo, apiInfo := getStateInfo(config, hostnames)
	return stateInfo, apiInfo, nil
}
