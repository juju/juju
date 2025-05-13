// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package assumes

import (
	"strings"

	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/core/semversion"
	chassumes "github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/testhelpers"
)

type SatCheckerSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&SatCheckerSuite{})

func (s *SatCheckerSuite) TestErrorReportingForSimpleExpression(c *tc.C) {
	fs := genFeatureSet(c)

	exprTree := mustParseAssumesExpr(c, `
assumes:
  - storage >= 2.19.43
  - storage < 2.20
`)

	expErr := `
Charm cannot be deployed because:
  - charm requires feature "storage" (version >= 2.19.43) but model currently supports version 2.19.42
`[1:]

	err := fs.Satisfies(exprTree)
	c.Assert(err, tc.Satisfies, IsRequirementsNotSatisfiedError, tc.Commentf("expected to get a RequirementsNotSatisfied error"))
	c.Assert(err.Error(), tc.Equals, expErr)
}

func (s *SatCheckerSuite) TestErrorReportingForCompositeExpressions(c *tc.C) {
	fs := genFeatureSet(c)

	exprTree := mustParseAssumesExpr(c, `
assumes:
  - any-of:
    - k8s-api >= 42
    - random-feature-a
  - all-of:
    - block-storage
    - juju >= 3
`)

	expErr := `
Charm cannot be deployed because:
  - charm requires at least one of the following:
    - charm requires Kubernetes version >= 42.0.0, model is running on version 1.18.0
    - charm requires feature "random-feature-a" but model does not support it
  - charm requires all of the following:
    - charm requires feature "block-storage" but model does not support it
    - charm requires Juju version >= 3.0.0, model has version 2.19.42
`[1:]

	err := fs.Satisfies(exprTree)
	c.Assert(err, tc.Satisfies, IsRequirementsNotSatisfiedError, tc.Commentf("expected to get a RequirementsNotSatisfied error"))
	c.Assert(err.Error(), tc.Equals, expErr)
}

func (s *SatCheckerSuite) TestErrorReportingForMultiLevelExpressionTree(c *tc.C) {
	fs := genFeatureSet(c)

	exprTree := mustParseAssumesExpr(c, `
assumes:
  - storage >= 42
  - storage < 45
  - any-of:
    - k8s-api >= 1.17
    - random-feature-a
  - all-of:
    - k8s-api >= 1.17
    - juju >= 3
    - any-of:
      - random-feature-b
      - random-feature-c
  - any-of:
    - random-feature-d
    - random-feature-e
`)

	expErr := `
Charm cannot be deployed because:
  - charm requires feature "storage" (version >= 42.0.0) but model currently supports version 2.19.42
  - charm requires all of the following:
    - charm requires Juju version >= 3.0.0, model has version 2.19.42
    - charm requires at least one of the following:
      - charm requires feature "random-feature-b" but model does not support it
      - charm requires feature "random-feature-c" but model does not support it
  - charm requires at least one of the following:
    - charm requires feature "random-feature-d" but model does not support it
    - charm requires feature "random-feature-e" but model does not support it
`[1:]

	err := fs.Satisfies(exprTree)
	c.Assert(err, tc.Satisfies, IsRequirementsNotSatisfiedError, tc.Commentf("expected to get a RequirementsNotSatisfied error"))
	c.Assert(err.Error(), tc.Equals, expErr)
}

func (s *SatCheckerSuite) TestAssumesExpressionSatisfied(c *tc.C) {
	fs := genFeatureSet(c)

	exprTree := mustParseAssumesExpr(c, `
assumes:
  - storage >= 2
  - storage < 3
  - any-of:
    - k8s-api >= 1.17
    - load-balancer
  - all-of:
    - juju >= 2
    - juju < 3
    - storage
`)

	err := fs.Satisfies(exprTree)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("expected assumes expression tree to be satisfied"))
}

func genFeatureSet(c *tc.C) FeatureSet {
	var fs FeatureSet
	fs.Add(
		Feature{
			Name:        "k8s-api",
			Description: "access to the kubernetes API",
			Version:     mustParseVersion(c, "1.18"),
		},
		Feature{
			Name:        "juju",
			Description: "version of Juju running on the controller",
			Version:     mustParseVersion(c, "2.19.42"),
		},
		Feature{
			Name:        "storage",
			Description: "create and manipulate storage primitives",
			Version:     mustParseVersion(c, "2.19.42"),
		},
		Feature{
			Name:        "load-balancer",
			Description: "create and manipulate load-balancers",
		},
	)

	return fs
}

func mustParseVersion(c *tc.C, verStr string) *semversion.Number {
	ver, err := semversion.ParseNonStrict(verStr)
	c.Assert(err, tc.ErrorIsNil)
	return &ver
}

func mustParseAssumesExpr(c *tc.C, exprYAML string) *chassumes.ExpressionTree {
	var payload = struct {
		ExprTree chassumes.ExpressionTree `yaml:"assumes"`
	}{}

	err := yaml.NewDecoder(strings.NewReader(exprYAML)).Decode(&payload)
	c.Assert(err, tc.ErrorIsNil)

	return &payload.ExprTree
}
