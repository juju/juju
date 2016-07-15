// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

// +build windows

package featureflag

import (
	"github.com/gabriel-samfira/sys/windows/registry"
)

// SetFlagsFromRegistry populates the global set from the registry keys on
// windows.
// White space between flags is ignored, and the flags are lower cased. Under
// normal circumstances this method is only ever called from the init
// function.
//
// NOTE: since SetFlagsFromRegistry should only ever called during the
// program startup (or tests), and it is serialized by the runtime, we don't
// use any mutux when setting the flag set.  Should this change in the future,
// a mutex should be used.
func SetFlagsFromRegistry(regVarKey string, regVarName string) {
	setFlags(getFlagsFromRegistry(regVarKey, regVarName))
}

// getFlagsFromRegistry returns the string value from a registry key
func getFlagsFromRegistry(regVarKey, regVarName string) string {
	regKeyPath := regVarKey[len(`HKLM:\`):]
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, regKeyPath, registry.QUERY_VALUE)
	if err != nil {
		// Since this is called during init, we can't fail here. We just log
		// the failure and move on.
		logger.Warningf("Failed to open juju registry key %s; feature flags not enabled", regVarKey)
		return ""
	}
	defer k.Close()

	f, _, err := k.GetStringValue(regVarName)
	if err != nil {
		// Since this is called during init, we can't fail here. We just log
		// the failure and move on.
		logger.Warningf("Failed to read juju registry value %s; feature flags not enabled", regVarName)
		return ""
	}

	return f
}
