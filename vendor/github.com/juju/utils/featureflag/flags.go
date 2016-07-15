// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// The featureflag package gives other parts of Juju the ability to easily
// check to see if a feature flag has been defined. Feature flags give the
// Juju developers a way to add new commands or features to Juju but hide
// them from the general user-base while things are still in development.
//
// Feature flags are defined through environment variables, and the value
// is comma separated.
//    # this defines two flags: "special" and "magic"
//    export JUJU_SOME_ENV_VAR=special,magic
//
// The feature flags should be read and identified at program initialisation
// time using an init function.  This function should call the
// `SetFlagsFromEnvironment` or the `SetFlagsFromRegistry` function.
package featureflag

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/juju/loggo"
	"github.com/juju/utils/set"
)

var (
	logger   = loggo.GetLogger("utils.featureflag")
	flaglock sync.Mutex // seralises access to flags
	flags    = set.NewStrings()
)

// SetFlagsFromEnvironment populates the global set from the environment.
// White space between flags is ignored, and the flags are lower cased. Under
// normal circumstances this method is only ever called from the init
// function.
//
// NOTE: since SetFlagsFromEnvironment should only ever called during the
// program startup (or tests), and it is serialized by the runtime, we don't
// use any mutux when setting the flag set.  Should this change in the future,
// a mutex should be used.
func SetFlagsFromEnvironment(envVarName string) {
	setFlags(os.Getenv(envVarName))
}

// setFlags populates the global set using a string passed to it containing the
// flags.
func setFlags(val string) {
	values := strings.ToLower(val)

	flaglock.Lock()
	defer flaglock.Unlock()
	flags = set.NewStrings()
	for _, flag := range strings.Split(values, ",") {
		if flag = strings.TrimSpace(flag); flag != "" {
			flags.Add(flag)
		}
	}
}

// Enabled is used to determine if a particular feature flag is enabled for
// the process.
func Enabled(flag string) bool {
	flaglock.Lock()
	defer flaglock.Unlock()
	flag = strings.TrimSpace(strings.ToLower(flag))
	if flag == "" {
		// The empty feature is always enabled.
		return true
	}
	return flags.Contains(flag)
}

// All returns all the current feature flags.
func All() []string {
	flaglock.Lock()
	defer flaglock.Unlock()
	return flags.Values()
}

// AsEnvironmentValue returns a single string suitable to be assigned into an
// environment value that will be parsed into the same set of values currently
// set.
func AsEnvironmentValue() string {
	flaglock.Lock()
	defer flaglock.Unlock()
	return strings.Join(flags.SortedValues(), ",")
}

// String provides a nice human readable string for the feature flags that
// are set.
func String() string {
	flaglock.Lock()
	defer flaglock.Unlock()
	var quoted []string
	for _, flag := range flags.SortedValues() {
		quoted = append(quoted, fmt.Sprintf("%q", flag))
	}
	return strings.Join(quoted, ", ")
}
