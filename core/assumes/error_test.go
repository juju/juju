// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package assumes

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
)

type errorSuite struct{}

func TestErrorSuite(t *testing.T) {
	tc.Run(t, &errorSuite{})
}

var errorTests = []struct {
	description string
	featureSet  FeatureSet
	assumes     string
	expectedErr string
}{{
	description: "Unsupported Juju version",
	featureSet: FeatureSet{features: map[string]Feature{
		"juju": JujuFeature(semversion.MustParse("2.9.42")),
	}},
	assumes:     "assumes:\n- juju >= 3.1",
	expectedErr: "(?s).*charm requires Juju version >= 3.1.0.*",
}, {
	description: "Deploying k8s charm on machine cloud",
	featureSet:  FeatureSet{},
	assumes:     "assumes:\n- k8s-api",
	expectedErr: "(?s).*charm must be deployed on a Kubernetes cloud.*",
}, {
	description: "k8s version too low",
	featureSet: FeatureSet{features: map[string]Feature{
		"k8s-api": K8sAPIFeature(semversion.MustParse("1.25.0")),
	}},
	assumes:     "assumes:\n- k8s-api >= 1.30",
	expectedErr: "(?s).*charm requires Kubernetes version >= 1.30.*",
}}

func (*errorSuite) TestErrorMessages(c *tc.C) {
	for _, test := range errorTests {
		fmt.Println(test.description)
		assumesTree := mustParseAssumesExpr(c, test.assumes)
		err := test.featureSet.Satisfies(assumesTree)
		c.Check(err, tc.ErrorMatches, test.expectedErr)
	}
}
