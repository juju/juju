// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/storage"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type PoolListSuite struct {
	SubStorageSuite
	mockAPI *mockPoolListAPI
}

var _ = gc.Suite(&PoolListSuite{})

func (s *PoolListSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockPoolListAPI{}
	s.PatchValue(storage.GetPoolListAPI, func(c *storage.PoolListCommand) (storage.PoolListAPI, error) {
		return s.mockAPI, nil
	})

}

func runPoolList(c *gc.C, args []string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&storage.PoolListCommand{}), args...)
}

func (s *PoolListSuite) TestPoolListEmpty(c *gc.C) {
	// Both arguments - names and provider types - are optional.
	// When none are supplied, all registered pools are listed.
	// As this test uses mock api, no pools are registered by default.
	// Returned list should be empty.
	s.assertValidList(
		c,
		[]string{""},
		"",
	)
}

func (s *PoolListSuite) TestPoolList(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--provider", "a", "--provider", "b", "--name", "xyz", "--name", "abc"},
		// Default format is yaml
		`
abc:
  provider: testType
  attrs:
    one: true
    three: maybe
    two: well
testName0:
  provider: a
  attrs:
    one: true
    three: maybe
    two: well
testName1:
  provider: b
  attrs:
    one: true
    three: maybe
    two: well
xyz:
  provider: testType
  attrs:
    one: true
    three: maybe
    two: well
`[1:],
	)
}

func (s *PoolListSuite) TestPoolListNoType(c *gc.C) {
	s.mockAPI.emptyType = true
	s.assertValidList(
		c,
		[]string{"--provider", "a", "--provider", "b", "--name", "xyz", "--name", "abc"},
		// Default format is yaml
		`
abc:
  attrs:
    one: true
    three: maybe
    two: well
testName0:
  attrs:
    one: true
    three: maybe
    two: well
testName1:
  attrs:
    one: true
    three: maybe
    two: well
xyz:
  attrs:
    one: true
    three: maybe
    two: well
`[1:],
	)
}

func (s *PoolListSuite) TestPoolListNoCfg(c *gc.C) {
	s.mockAPI.emptyConfig = true
	s.assertValidList(
		c,
		[]string{"--provider", "a", "--provider", "b", "--name", "xyz", "--name", "abc"},
		// Default format is yaml
		`
abc:
  provider: testType
testName0:
  provider: a
testName1:
  provider: b
xyz:
  provider: testType
`[1:],
	)
}

func (s *PoolListSuite) TestPoolListJSON(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--provider", "a", "--provider", "b",
			"--name", "xyz", "--name", "abc",
			"--format", "json"},
		`{"abc":{"provider":"testType","attrs":{"one":true,"three":"maybe","two":"well"}},"testName0":{"provider":"a","attrs":{"one":true,"three":"maybe","two":"well"}},"testName1":{"provider":"b","attrs":{"one":true,"three":"maybe","two":"well"}},"xyz":{"provider":"testType","attrs":{"one":true,"three":"maybe","two":"well"}}}
`,
	)
}

func (s *PoolListSuite) TestPoolListTabular(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--provider", "a", "--provider", "b",
			"--name", "xyz", "--name", "abc",
			"--format", "tabular"},
		`
NAME       PROVIDER  ATTRS
abc        testType  one=true,two=well,three=maybe
testName0  a         one=true,two=well,three=maybe
testName1  b         one=true,two=well,three=maybe
xyz        testType  one=true,two=well,three=maybe

`[1:])
}

func (s *PoolListSuite) TestPoolListTabularSorted(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--name", "myaw", "--name", "xyz", "--name", "abc",
			"--format", "tabular"},
		`
NAME  PROVIDER  ATTRS
abc   testType  one=true,two=well,three=maybe
myaw  testType  one=true,two=well,three=maybe
xyz   testType  one=true,two=well,three=maybe

`[1:])
}

func (s *PoolListSuite) assertValidList(c *gc.C, args []string, expected string) {
	context, err := runPoolList(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtained := testing.Stdout(context)
	c.Assert(obtained, gc.Equals, expected)
}

type mockPoolListAPI struct {
	emptyConfig, emptyType bool
}

func (s mockPoolListAPI) Close() error {
	return nil
}

func (s mockPoolListAPI) ListPools(types []string, names []string) ([]params.StoragePool, error) {
	results := make([]params.StoragePool, len(types)+len(names))
	var index int
	addInstance := func(aname, atype string) {
		results[index] = s.createTestPoolInstance(aname, atype)
		index++
	}
	for i, atype := range types {
		addInstance(fmt.Sprintf("testName%v", i), atype)
	}
	for _, aname := range names {
		addInstance(aname, "testType")
	}
	return results, nil
}

func (s mockPoolListAPI) createTestPoolInstance(aname, atype string) params.StoragePool {
	if s.emptyType {
		atype = ""
	}
	attrs := make(map[string]interface{})
	if !s.emptyConfig {
		attrs = map[string]interface{}{"one": true, "two": "well", "three": "maybe"}
	}
	return params.StoragePool{
		Name:     aname,
		Provider: atype,
		Attrs:    attrs,
	}
}
