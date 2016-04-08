// Copyright 2012, 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build windows

package testing

import (
	"github.com/gabriel-samfira/sys/windows/registry"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
)

// setUpFeatureFlags sets the feature flags from the environment, but
// also sets up the registry key on windows because some functions are
// trying to look in there.
func (s *JujuOSEnvSuite) setUpFeatureFlags(c *gc.C) {
	const len = len(`HKLM:\`)
	regKey := osenv.JujuRegistryKey[len:]

	k, exists := createKey(c, regKey)
	defer closeKey(c, k)

	s.regKeyExisted = exists
	s.regEntryExisted = false

	val, _, err := k.GetStringValue(osenv.JujuFeatureFlagEnvKey)
	if errors.Cause(err) != registry.ErrNotExist {
		c.Assert(err, jc.ErrorIsNil)
		s.oldRegEntryValue = val
		s.regEntryExisted = true
	}

	err = k.SetStringValue(osenv.JujuFeatureFlagEnvKey, s.initialFeatureFlags)
	c.Assert(err, jc.ErrorIsNil)
	featureflag.SetFlagsFromRegistry(osenv.JujuRegistryKey, osenv.JujuFeatureFlagEnvKey)
}

// tearDownFeatureFlags restores the old registry values
func (s *JujuOSEnvSuite) tearDownFeatureFlags(c *gc.C) {
	const len = len(`HKLM:\`)
	regKey := osenv.JujuRegistryKey[len:]

	if s.regKeyExisted {
		k := openKey(c, regKey)
		defer closeKey(c, k)
		if s.regEntryExisted {
			err := k.SetStringValue(osenv.JujuFeatureFlagEnvKey, s.oldRegEntryValue)
			c.Assert(err, jc.ErrorIsNil)
		} else {
			err := k.DeleteValue(osenv.JujuFeatureFlagEnvKey)
			c.Assert(err, jc.ErrorIsNil)
		}
	} else {
		deleteKey(c, regKey)
	}
}

func createKey(c *gc.C, regKey string) (registry.Key, bool) {
	k, exists, err := registry.CreateKey(registry.LOCAL_MACHINE, regKey, registry.ALL_ACCESS)
	c.Assert(err, jc.ErrorIsNil)
	return k, exists
}

func openKey(c *gc.C, regKey string) registry.Key {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, regKey, registry.ALL_ACCESS)
	c.Assert(err, jc.ErrorIsNil)
	return k
}

func closeKey(c *gc.C, k registry.Key) {
	err := k.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func deleteKey(c *gc.C, regKey string) {
	err := registry.DeleteKey(registry.LOCAL_MACHINE, regKey)
	c.Assert(err, jc.ErrorIsNil)
}
