// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rules

type Report struct {
	Manifolds map[string]Worker `yaml:"manifolds"`
}

type Worker struct {
	Inputs     []string     `yaml:"inputs"`
	Report     WorkerReport `yaml:"report"`
	StartCount int          `yaml:"start-count"`
	Started    string       `yaml:"started"`
	State      string       `yaml:"state"`
}

type WorkerReport struct {
	Agent   string                  `yaml:"agent"`
	State   string                  `yaml:"state"`
	Targets map[string]WorkerTarget `yaml:"targets"`
}

type WorkerTarget struct {
	Status string
}
