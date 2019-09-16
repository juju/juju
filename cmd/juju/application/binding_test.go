// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/collections/set"
	"github.com/juju/juju/core/network"
	"github.com/juju/testing"

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
	knownSpaces := []string{"a", "b"}
	s.checkParseOKForExpr(c, "foo=a bar=b", knownSpaces, map[string]string{"foo": "a", "bar": "b"})
}

func (s *ParseBindSuite) TestParseSuccessWithApplicationDefaultSpaceOnly(c *gc.C) {
	knownSpaces := []string{"application-default"}
	s.checkParseOKForExpr(c, "application-default", knownSpaces, map[string]string{"": "application-default"})
}

func (s *ParseBindSuite) TestBindingsOrderForDefaultSpaceAndEndpointsDoesNotMatter(c *gc.C) {
	knownSpaces := []string{"sp3", "sp1", "sp2"}
	expectedBindings := map[string]string{
		"ep1": "sp1",
		"ep2": "sp2",
		"":    "sp3",
	}
	s.checkParseOKForExpr(c, "ep1=sp1 ep2=sp2 sp3", knownSpaces, expectedBindings)
	s.checkParseOKForExpr(c, "ep1=sp1 sp3 ep2=sp2", knownSpaces, expectedBindings)
	s.checkParseOKForExpr(c, "ep2=sp2 ep1=sp1 sp3", knownSpaces, expectedBindings)
	s.checkParseOKForExpr(c, "ep2=sp2 sp3 ep1=sp1", knownSpaces, expectedBindings)
	s.checkParseOKForExpr(c, "sp3 ep1=sp1 ep2=sp2", knownSpaces, expectedBindings)
	s.checkParseOKForExpr(c, "sp3 ep2=sp2 ep1=sp1", knownSpaces, expectedBindings)
}

func (s *ParseBindSuite) TestParseWithEmptyQuotedDefaultSpace(c *gc.C) {
	knownSpaces := []string{"", "sp1"}
	expectedBindings := map[string]string{
		"ep1": "sp1",
		"ep2": "",
		"":    "",
	}
	s.checkParseOKForExpr(c, `"" ep2="" ep1=sp1`, knownSpaces, expectedBindings)
}

func (s *ParseBindSuite) TestParseFailsWithSpaceNameButNoEndpoint(c *gc.C) {
	s.checkParseFailsForExpr(c, "=bad", nil, "Found = without endpoint name. Use a lone space name to set the default.")
}

func (s *ParseBindSuite) TestParseFailsWithTooManyEqualsSignsInArgs(c *gc.C) {
	s.checkParseFailsForExpr(c, "foo=bar=baz", nil, "Found multiple = in binding. Did you forget to space-separate the binding list?")
}

func (s *ParseBindSuite) TestParseFailsWithUnknownSpaceName(c *gc.C) {
	_, err := parseBindExpr("rel1=bogus", nil)
	c.Check(err.Error(), gc.Equals, `Space with name "bogus" not found`)
}

func (s *ParseBindSuite) TestMergeBindingsNewBindingsInheritDefaultSpace(c *gc.C) {
	newCharmEndpoints := set.NewStrings("ep1", "ep2", "ep3")
	oldEndpointsMap := map[string]string{
		"":    network.DefaultSpaceName,
		"ep1": "sp1",
	}

	userBindings := map[string]string{
		"ep1": "sp-foo", // overwrite existing space assignment
		"ep3": "sp1",    // set space for new endpoint
	}

	expMergedBindings := map[string]string{
		"ep1": "sp-foo",
		"ep2": network.DefaultSpaceName, // new endpoint ep2 inherits the default space
		"ep3": "sp1",
	}

	mergedBindings, err := mergeBindings(newCharmEndpoints, oldEndpointsMap, userBindings, network.DefaultSpaceName)
	c.Assert(err, gc.IsNil)
	c.Assert(mergedBindings, gc.DeepEquals, expMergedBindings)
}

func (s *ParseBindSuite) checkParseOKForExpr(c *gc.C, expr string, knownSpaces []string, expectedBindings map[string]string) {
	parsedBindings, err := parseBindExpr(expr, knownSpaces)
	c.Check(err, jc.ErrorIsNil)
	c.Check(parsedBindings, jc.DeepEquals, expectedBindings)
}

func (s *ParseBindSuite) checkParseFailsForExpr(c *gc.C, expr string, knownSpaces []string, expectedErrorSuffix string) {
	parsedBindings, err := parseBindExpr(expr, knownSpaces)
	c.Check(err.Error(), gc.Equals, parseBindErrorPrefix+expectedErrorSuffix)
	c.Check(parsedBindings, gc.IsNil)
}
