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
	regKey := osenv.JujuRegistryKey[len(`HKLM:\`):]
	k, existed, err := registry.CreateKey(registry.LOCAL_MACHINE, regKey, registry.ALL_ACCESS)
	s.regKeyExisted = existed
	c.Assert(err, jc.ErrorIsNil)
	val, _, err := k.GetStringValue(osenv.JujuFeatureFlagEnvKey)
	if errors.Cause(err) == registry.ErrNotExist {
		s.regEntryExisted = false
	} else {
		c.Assert(err, jc.ErrorIsNil)
		s.oldRegEntryValue = val
		s.regEntryExisted = true
	}
	err = k.SetStringValue(osenv.JujuFeatureFlagEnvKey, s.initialFeatureFlags)
	c.Assert(err, jc.ErrorIsNil)
	featureflag.SetFlagsFromRegistry(osenv.JujuRegistryKey, osenv.JujuFeatureFlagEnvKey)
	err = k.Close()
	c.Assert(err, jc.ErrorIsNil)

}

// tearDownFeatureFlags restores the old registry values
func (s *JujuOSEnvSuite) tearDownFeatureFlags(c *gc.C) {
	regKey := osenv.JujuRegistryKey[len(`HKLM:\`):]
	if s.regKeyExisted {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, regKey, registry.ALL_ACCESS)
		if s.regEntryExisted {
			err := k.SetStringValue(osenv.JujuFeatureFlagEnvKey, s.oldRegEntryValue)
			c.Assert(err, jc.ErrorIsNil)
		} else {
			err := k.DeleteValue(osenv.JujuFeatureFlagEnvKey)
			c.Assert(err, jc.ErrorIsNil)
		}
		err = k.Close()
		c.Assert(err, jc.ErrorIsNil)
	} else {
		err := registry.DeleteKey(registry.LOCAL_MACHINE, regKey)
		c.Assert(err, jc.ErrorIsNil)
	}

}
