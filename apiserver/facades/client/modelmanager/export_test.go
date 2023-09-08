// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/state/stateenvirons"
)

func AuthCheck(c *gc.C, mm *ModelManagerAPI, user names.UserTag) bool {
	err := mm.authCheck(user)
	c.Assert(err, jc.ErrorIsNil)
	return mm.isAdmin
}

func MockSupportedFeatures(fs assumes.FeatureSet) {
	supportedFeaturesGetter = func(stateenvirons.Model, stateenvirons.CloudService, stateenvirons.CredentialService) (assumes.FeatureSet, error) {
		return fs, nil
	}
}

func ResetSupportedFeaturesGetter() {
	supportedFeaturesGetter = stateenvirons.SupportedFeatures
}
