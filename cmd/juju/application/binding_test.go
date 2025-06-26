// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
)

type ParseBindSuite struct {
	testhelpers.LoggingSuite
}

func TestParseBindSuite(t *testing.T) {
	tc.Run(t, &ParseBindSuite{})
}

func (s *ParseBindSuite) SetUpSuite(c *tc.C) {
	s.LoggingSuite.SetUpSuite(c)
}

func (s *ParseBindSuite) TearDownSuite(c *tc.C) {
	s.LoggingSuite.TearDownSuite(c)
}

func (s *ParseBindSuite) TestParseSuccessWithEmptyArgs(c *tc.C) {
	s.checkParseOKForExpr(c, "", nil, nil)
}

func (s *ParseBindSuite) TestParseSuccessWithEndpointsOnly(c *tc.C) {
	knownSpaceNames := set.NewStrings("a", "b")
	s.checkParseOKForExpr(c, "foo=a bar=b", knownSpaceNames, map[string]string{"foo": "a", "bar": "b"})
}

func (s *ParseBindSuite) TestParseSuccessWithApplicationDefaultSpaceOnly(c *tc.C) {
	knownSpaceNames := set.NewStrings("application-default")
	s.checkParseOKForExpr(c, "application-default", knownSpaceNames, map[string]string{"": "application-default"})
}

func (s *ParseBindSuite) TestBindingsOrderForDefaultSpaceAndEndpointsDoesNotMatter(c *tc.C) {
	knownSpaceNames := set.NewStrings("sp3", "sp1", "sp2")
	expectedBindings := map[string]string{
		"ep1": "sp1",
		"ep2": "sp2",
		"":    "sp3",
	}
	s.checkParseOKForExpr(c, "ep1=sp1 ep2=sp2 sp3", knownSpaceNames, expectedBindings)
	s.checkParseOKForExpr(c, "ep1=sp1 sp3 ep2=sp2", knownSpaceNames, expectedBindings)
	s.checkParseOKForExpr(c, "ep2=sp2 ep1=sp1 sp3", knownSpaceNames, expectedBindings)
	s.checkParseOKForExpr(c, "ep2=sp2 sp3 ep1=sp1", knownSpaceNames, expectedBindings)
	s.checkParseOKForExpr(c, "sp3 ep1=sp1 ep2=sp2", knownSpaceNames, expectedBindings)
	s.checkParseOKForExpr(c, "sp3 ep2=sp2 ep1=sp1", knownSpaceNames, expectedBindings)
}

func (s *ParseBindSuite) TestParseWithEmptyQuotedDefaultSpace(c *tc.C) {
	knownSpaceNames := set.NewStrings("", "sp1")
	expectedBindings := map[string]string{
		"ep1": "sp1",
		"ep2": "",
		"":    "",
	}
	s.checkParseOKForExpr(c, `"" ep2="" ep1=sp1`, knownSpaceNames, expectedBindings)
}

func (s *ParseBindSuite) TestParseFailsWithSpaceNameButNoEndpoint(c *tc.C) {
	s.checkParseFailsForExpr(c, "=bad", nil, fmt.Sprintf(parseBindError,
		`found "=" without endpoint name. Use a lone space name to set the default.`))
}

func (s *ParseBindSuite) TestParseFailsWithTooManyEqualsSignsInArgs(c *tc.C) {
	s.checkParseFailsForExpr(c, "foo=bar=baz", nil, fmt.Sprintf(parseBindError,
		`found multiple "=" in binding. Did you forget to space-separate the binding list?`))
}

func (s *ParseBindSuite) TestParseFailsWithUnknownSpaceName(c *tc.C) {
	_, err := parseBindExpr("rel1=bogus", nil)
	c.Check(err.Error(), tc.Equals, `space "bogus" not found`)
}

func (s *ParseBindSuite) TestMergeBindingsNewBindingsInheritDefaultSpace(c *tc.C) {
	newCharmEndpoints := set.NewStrings("ep1", "ep2", "ep3", "ep4", "ep5")
	oldEndpointsMap := map[string]string{
		"":    network.AlphaSpaceName.String(),
		"ep1": "sp1",
		"ep4": network.AlphaSpaceName.String(),
		"ep5": network.AlphaSpaceName.String(),
	}

	userBindings := map[string]string{
		"ep1": "sp-foo", // overwrite existing space assignment
		"ep3": "sp1",    // set space for new endpoint
	}

	expMergedBindings := map[string]string{
		"ep1": "sp-foo",
		"ep2": network.AlphaSpaceName.String(), // new endpoint ep2 inherits the default space
		"ep3": "sp1",
		"ep4": network.AlphaSpaceName.String(),
		"ep5": network.AlphaSpaceName.String(),
	}

	mergedBindings, changeLog := mergeBindings(newCharmEndpoints, oldEndpointsMap, userBindings, network.AlphaSpaceName.String())
	c.Check(mergedBindings, tc.DeepEquals, expMergedBindings)
	c.Check(changeLog, tc.SameContents, []string{
		`moving endpoint "ep1" from space "sp1" to "sp-foo"`,
		`adding endpoint "ep2" to default space "alpha"`,
		`adding endpoint "ep3" to space "sp1"`,
		`no change to endpoints in space "alpha": ep4, ep5`,
	})
}

func (s *ParseBindSuite) checkParseOKForExpr(c *tc.C, expr string, knownSpaceNames set.Strings, expectedBindings map[string]string) {
	parsedBindings, err := parseBindExpr(expr, knownSpaceNames)
	c.Check(err, tc.ErrorIsNil)
	c.Check(parsedBindings, tc.DeepEquals, expectedBindings)
}

func (s *ParseBindSuite) checkParseFailsForExpr(c *tc.C, expr string, knownSpaceNames set.Strings, expectedErrorSuffix string) {
	parsedBindings, err := parseBindExpr(expr, knownSpaceNames)
	c.Check(err.Error(), tc.Equals, expectedErrorSuffix)
	c.Check(parsedBindings, tc.IsNil)
}
