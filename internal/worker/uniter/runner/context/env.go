// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"
	"os"

	jujuos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
)

// Environmenter represent the os environ interface for fetching host level environment
// variables.
type Environmenter interface {
	// Environ returns a copy of strings representing the environment, in the
	// form "key=value"
	Environ() []string

	// Getenv retrieves the value of the environment variable named by the key.
	// It returns the value, which will be empty if the variable is not present.
	Getenv(string) string

	// LookupEnv retrieves the value of the environment variable named by the
	// key. If the variable is present in the environment the value (which may
	// be empty) is returned and the boolean is true. Otherwise the returned
	// value will be empty and the boolean will be false.
	LookupEnv(string) (string, bool)
}

type EnvironmentWrapper struct {
	environFn   func() []string
	getenvFn    func(string) string
	lookupEnvFn func(string) (string, bool)
}

// ContextAllowedEnvVars defines a list of allowed env vars to include from the
// given context
var ContextAllowedEnvVars = []string{
	// List of Kubernetes env vars to include
	"KUBERNETES_PORT",
	"KUBERNETES_PORT_443_TCP",
	"KUBERNETES_PORT_443_TCP_ADDR",
	"KUBERNETES_PORT_443_TCP_PORT",
	"KUBERNETES_PORT_443_TCP_PROTO",
	"KUBERNETES_SERVICE",
	"KUBERNETES_SERVICE_HOST",
	"KUBERNETES_SERVICE_PORT",
	"KUBERNETES_SERVICE_PORT_HTTPS",
}

// NewHostEnvironmenter constructs an EnvironmentWrapper target at the current
// process host
func NewHostEnvironmenter() *EnvironmentWrapper {
	return &EnvironmentWrapper{
		environFn:   os.Environ,
		getenvFn:    os.Getenv,
		lookupEnvFn: os.LookupEnv,
	}
}

// NewRemoveEnvironmenter constructs an EnviornmentWrapper with targets set to
// that of the functions provided.
func NewRemoteEnvironmenter(
	environFn func() []string,
	getenvFn func(string) string,
	lookupEnvFn func(string) (string, bool),
) *EnvironmentWrapper {
	return &EnvironmentWrapper{
		environFn:   environFn,
		getenvFn:    getenvFn,
		lookupEnvFn: lookupEnvFn,
	}
}

// Environ implements Environmenter Environ
func (e *EnvironmentWrapper) Environ() []string {
	return e.environFn()
}

// Getenv implements Environmenter Getenv
func (e *EnvironmentWrapper) Getenv(key string) string {
	return e.getenvFn(key)
}

// LookupEnv implements Environmenter LookupEnv
func (e *EnvironmentWrapper) LookupEnv(key string) (string, bool) {
	return e.lookupEnvFn(key)
}

// ContextDependentEnvVars returns the context aware enviormnent variables
// needed for charms depending on contexts they may be operating in.
// For example returning defined Kubernetes env variables when they're defined.
func ContextDependentEnvVars(env Environmenter) []string {
	rval := make([]string, 0, len(ContextAllowedEnvVars))
	for _, envKey := range ContextAllowedEnvVars {
		if val, exists := env.LookupEnv(envKey); exists {
			rval = append(rval, fmt.Sprintf("%s=%s", envKey, val))
		}
	}
	return rval
}

// OSDependentEnvVars returns the OS-dependent environment variables that
// should be set for a hook context.
func OSDependentEnvVars(paths Paths, env Environmenter) []string {
	switch jujuos.HostOS() {
	case ostype.Ubuntu:
		return ubuntuEnv(paths, env)
	}
	return nil
}

func appendPath(paths Paths, env Environmenter) []string {
	return []string{
		"PATH=" + paths.GetToolsDir() + ":" + env.Getenv("PATH"),
	}
}

func ubuntuEnv(paths Paths, envVars Environmenter) []string {
	path := appendPath(paths, envVars)
	env := []string{
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
		"LANG=C.UTF-8",
		"TERM=tmux-256color",
	}
	return append(env, path...)
}
