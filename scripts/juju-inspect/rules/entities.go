// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rules

import "gopkg.in/yaml.v3"

type Report struct {
	Manifolds map[string]Worker `yaml:"manifolds"`
}

type Worker struct {
	Inputs     []string    `yaml:"inputs"`
	Report     interface{} `yaml:"report"`
	StartCount int         `yaml:"start-count"`
	Started    string      `yaml:"started"`
	State      string      `yaml:"state"`
}

func (w Worker) UnmarshalReport(out interface{}) error {
	b, err := yaml.Marshal(w.Report)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, out)
}
