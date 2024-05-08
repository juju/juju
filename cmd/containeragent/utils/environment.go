// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import "os"

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/environment_mock.go github.com/juju/juju/cmd/containeragent/utils Environment

// Environment provides methods for reading a file or writing to a file.
type Environment interface {
	// Setenv changes the environment variable k to value v.
	Setenv(k, v string) error
	// Unsetenv deletes the environment variable k.
	Unsetenv(k string) error
	// ExpandEnv replaces $VAR and ${VAR} in string s with values from the environment.
	ExpandEnv(s string) string
	// Getenv gets an environment variable from the environment.
	Getenv(k string) string
}

// NewEnvironment returns a new Environment.
func NewEnvironment() Environment {
	return &environment{}
}

type environment struct{}

var _ Environment = (*environment)(nil)

// Setenv changes the environment variable k to value v.
func (environment) Setenv(k, v string) error {
	return os.Setenv(k, v)
}

// Unsetenv deletes the environment variable k.
func (environment) Unsetenv(k string) error {
	return os.Unsetenv(k)
}

// ExpandEnv replaces $VAR and ${VAR} in string s with values from the environment.
func (environment) ExpandEnv(s string) string {
	return os.ExpandEnv(s)
}

// Getenv gets an environment variable from the environment.
func (environment) Getenv(k string) string {
	return os.Getenv(k)
}
