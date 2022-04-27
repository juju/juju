// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/collections/set"
	"github.com/juju/testing"

	"github.com/juju/juju/core/network"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type ParseBindSuite struct {
	testing.LoggingSuite
}

var _ = gc.Suite(&ParseBindSuite{})

func (s *ParseBindSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
}

func (s *ParseBindSuite) TearDownSuite(c *gc.C) {
	s.LoggingSuite.TearDownSuite(c)
}

func (s *ParseBindSuite) TestParseSuccessWithEmptyArgs(c *gc.C) {
	s.checkParseOKForExpr(c, "", nil, nil)
}

func (s *ParseBindSuite) TestParseSuccessWithEndpointsOnly(c *gc.C) {
	knownSpaceNames := set.NewStrings("a", "b")
	s.checkParseOKForExpr(c, "foo=a bar=b", knownSpaceNames, map[string]string{"foo": "a", "bar": "b"})
}

func (s *ParseBindSuite) TestParseSuccessWithApplicationDefaultSpaceOnly(c *gc.C) {
	knownSpaceNames := set.NewStrings("application-default")
	s.checkParseOKForExpr(c, "application-default", knownSpaceNames, map[string]string{"": "application-default"})
}

func (s *ParseBindSuite) TestBindingsOrderForDefaultSpaceAndEndpointsDoesNotMatter(c *gc.C) {
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

func (s *ParseBindSuite) TestParseWithEmptyQuotedDefaultSpace(c *gc.C) {
	knownSpaceNames := set.NewStrings("", "sp1")
	expectedBindings := map[string]string{
		"ep1": "sp1",
		"ep2": "",
		"":    "",
	}
	s.checkParseOKForExpr(c, `"" ep2="" ep1=sp1`, knownSpaceNames, expectedBindings)
}

func (s *ParseBindSuite) TestParseFailsWithSpaceNameButNoEndpoint(c *gc.C) {
	s.checkParseFailsForExpr(c, "=bad", nil, parseBindErrorPrefix+"Found = without endpoint name. Use a lone space name to set the default.")
}

func (s *ParseBindSuite) TestParseFailsWithTooManyEqualsSignsInArgs(c *gc.C) {
	s.checkParseFailsForExpr(c, "foo=bar=baz", nil, parseBindErrorPrefix+"Found multiple = in binding. Did you forget to space-separate the binding list?")
}

func (s *ParseBindSuite) TestParseFailsWithUnknownSpaceName(c *gc.C) {
	_, err := parseBindExpr("rel1=bogus", nil)
	c.Check(err.Error(), gc.Equals, `space "bogus" not found`)
}

func (s *ParseBindSuite) TestMergeBindingsNewBindingsInheritDefaultSpace(c *gc.C) {
	newCharmEndpoints := set.NewStrings("ep1", "ep2", "ep3")
	oldEndpointsMap := map[string]string{
		"":    network.AlphaSpaceName,
		"ep1": "sp1",
	}

	userBindings := map[string]string{
		"ep1": "sp-foo", // overwrite existing space assignment
		"ep3": "sp1",    // set space for new endpoint
	}

	expMergedBindings := map[string]string{
		"ep1": "sp-foo",
		"ep2": network.AlphaSpaceName, // new endpoint ep2 inherits the default space
		"ep3": "sp1",
	}

	mergedBindings, _ := mergeBindings(newCharmEndpoints, oldEndpointsMap, userBindings, network.AlphaSpaceName)
	c.Assert(mergedBindings, gc.DeepEquals, expMergedBindings)
}

func (s *ParseBindSuite) checkParseOKForExpr(c *gc.C, expr string, knownSpaceNames set.Strings, expectedBindings map[string]string) {
	parsedBindings, err := parseBindExpr(expr, knownSpaceNames)
	c.Check(err, jc.ErrorIsNil)
	c.Check(parsedBindings, jc.DeepEquals, expectedBindings)
}

func (s *ParseBindSuite) checkParseFailsForExpr(c *gc.C, expr string, knownSpaceNames set.Strings, expectedErrorSuffix string) {
	parsedBindings, err := parseBindExpr(expr, knownSpaceNames)
	c.Check(err.Error(), gc.Equals, expectedErrorSuffix)
	c.Check(parsedBindings, gc.IsNil)
}
