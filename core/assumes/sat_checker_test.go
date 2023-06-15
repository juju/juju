// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package assumes

import (
	"strings"

	chassumes "github.com/juju/charm/v11/assumes"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v3"
)

type SatCheckerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SatCheckerSuite{})

func (s *SatCheckerSuite) TestErrorReportingForSimpleExpression(c *gc.C) {
	fs := genFeatureSet(c)

	exprTree := mustParseAssumesExpr(c, `
assumes:
  - storage >= 2.19.43
  - storage < 2.20
`)

	expErr := `
Charm feature requirements cannot be met:
  - charm requires feature "storage" (version >= 2.19.43) but model currently supports version 2.19.42

Feature descriptions:
  - "storage": create and manipulate storage primitives

For additional information please see: https://juju.is/docs/olm/supported-features`[1:]

	err := fs.Satisfies(exprTree)
	c.Assert(err, jc.Satisfies, IsRequirementsNotSatisfiedError, gc.Commentf("expected to get a RequirementsNotSatisfied error"))
	c.Assert(err.Error(), gc.Equals, expErr)
}

func (s *SatCheckerSuite) TestErrorReportingForCompositeExpressions(c *gc.C) {
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
Charm feature requirements cannot be met:
  - charm requires at least one of the following:
    - charm requires feature "k8s-api" (version >= 42.0.0) but model currently supports version 1.18.0
    - charm requires feature "random-feature-a" but model does not support it
  - charm requires all of the following:
    - charm requires feature "block-storage" but model does not support it
    - charm requires feature "juju" (version >= 3.0.0) but model currently supports version 2.19.42

Feature descriptions:
  - "juju": version of Juju running on the controller
  - "k8s-api": access to the kubernetes API

For additional information please see: https://juju.is/docs/olm/supported-features`[1:]

	err := fs.Satisfies(exprTree)
	c.Assert(err, jc.Satisfies, IsRequirementsNotSatisfiedError, gc.Commentf("expected to get a RequirementsNotSatisfied error"))
	c.Assert(err.Error(), gc.Equals, expErr)
}

func (s *SatCheckerSuite) TestErrorReportingForMultiLevelExpressionTree(c *gc.C) {
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
Charm feature requirements cannot be met:
  - charm requires feature "storage" (version >= 42.0.0) but model currently supports version 2.19.42
  - charm requires all of the following:
    - charm requires feature "juju" (version >= 3.0.0) but model currently supports version 2.19.42
    - charm requires at least one of the following:
      - charm requires feature "random-feature-b" but model does not support it
      - charm requires feature "random-feature-c" but model does not support it
  - charm requires at least one of the following:
    - charm requires feature "random-feature-d" but model does not support it
    - charm requires feature "random-feature-e" but model does not support it

Feature descriptions:
  - "juju": version of Juju running on the controller
  - "storage": create and manipulate storage primitives

For additional information please see: https://juju.is/docs/olm/supported-features`[1:]

	err := fs.Satisfies(exprTree)
	c.Assert(err, jc.Satisfies, IsRequirementsNotSatisfiedError, gc.Commentf("expected to get a RequirementsNotSatisfied error"))
	c.Assert(err.Error(), gc.Equals, expErr)
}

func (s *SatCheckerSuite) TestAssumesExpressionSatisfied(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("expected assumes expression tree to be satisfied"))
}

func genFeatureSet(c *gc.C) FeatureSet {
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

func mustParseVersion(c *gc.C, verStr string) *version.Number {
	ver, err := version.ParseNonStrict(verStr)
	c.Assert(err, jc.ErrorIsNil)
	return &ver
}

func mustParseAssumesExpr(c *gc.C, exprYAML string) *chassumes.ExpressionTree {
	var payload = struct {
		ExprTree chassumes.ExpressionTree `yaml:"assumes"`
	}{}

	err := yaml.NewDecoder(strings.NewReader(exprYAML)).Decode(&payload)
	c.Assert(err, jc.ErrorIsNil)

	return &payload.ExprTree
}
