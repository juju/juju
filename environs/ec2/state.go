package ec2

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
)

const stateFile = "provider-state"

type bootstrapState struct {
	StateInstances []string `yaml:"state-instances"`
}

func (e *environ) saveState(state *bootstrapState) error {
	data, err := goyaml.Marshal(state)
	if err != nil {
		return err
	}
	return e.Storage().Put(stateFile, bytes.NewBuffer(data), int64(len(data)))
}

func (e *environ) loadState() (*bootstrapState, error) {
	r, err := e.Storage().Get(stateFile)
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
