// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package featureflag gives other parts of Juju the ability to easily
// check to see if a feature flag has been defined. Feature flags give the
// Juju developers a way to add new commands or features to Juju but hide
// them from the general user-base while things are still in development.
//
// Feature flags are defined through environment variables, and the value
// is comma separated.
//
//	# this defines two flags: "special" and "magic"
//	export JUJU_SOME_ENV_VAR=special,magic
//
// The feature flags should be read and identified at program initialisation
// time using an init function.  This function should call the
// `SetFlagsFromEnvironment` or the `SetFlagsFromRegistry` function.
package featureflag
