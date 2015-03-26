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

type poolListSuite struct {
	SubStorageSuite
	mockAPI *mockPoolListAPI
}

var _ = gc.Suite(&poolListSuite{})

func (s *poolListSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockPoolListAPI{
		attrs: map[string]interface{}{"key": "value"},
	}
	s.PatchValue(storage.GetPoolListAPI,
		func(c *storage.PoolListCommand) (storage.PoolListAPI, error) {
			return s.mockAPI, nil
		})

}

func runPoolList(c *gc.C, args []string) (*cmd.Context, error) {
	return testing.RunCommand(c,
		envcmd.Wrap(&storage.PoolListCommand{}), args...)
}

func (s *poolListSuite) TestPoolListEmpty(c *gc.C) {
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

func (s *poolListSuite) TestPoolList(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--provider", "a",
			"--provider", "b",
			"--name", "xyz",
			"--name", "abc"},
		// Default format is yaml
		`
abc:
  provider: testType
  attrs:
    key: value
testName0:
  provider: a
  attrs:
    key: value
testName1:
  provider: b
  attrs:
    key: value
xyz:
  provider: testType
  attrs:
    key: value
`[1:],
	)
}

func (s *poolListSuite) TestPoolListJSON(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--provider", "a", "--provider", "b",
			"--name", "xyz", "--name", "abc",
			"--format", "json"},
		`{"abc":{"provider":"testType","attrs":{"key":"value"}},"testName0":{"provider":"a","attrs":{"key":"value"}},"testName1":{"provider":"b","attrs":{"key":"value"}},"xyz":{"provider":"testType","attrs":{"key":"value"}}}
`,
	)
}

func (s *poolListSuite) TestPoolListTabular(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--provider", "a", "--provider", "b",
			"--name", "xyz", "--name", "abc",
			"--format", "tabular"},
		`
NAME       PROVIDER  ATTRS
abc        testType  key=value
testName0  a         key=value
testName1  b         key=value
xyz        testType  key=value

`[1:])
}

func (s *poolListSuite) TestPoolListTabularSortedWithAttrs(c *gc.C) {
	s.mockAPI.attrs = map[string]interface{}{
		"a": true, "c": "well", "b": "maybe"}

	s.assertValidList(
		c,
		[]string{"--name", "myaw", "--name", "xyz", "--name", "abc",
			"--format", "tabular"},
		`
NAME  PROVIDER  ATTRS
abc   testType  a=true b=maybe c=well
myaw  testType  a=true b=maybe c=well
xyz   testType  a=true b=maybe c=well

`[1:])
}

func (s *poolListSuite) assertValidList(c *gc.C, args []string, expected string) {
	context, err := runPoolList(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtained := testing.Stdout(context)
	c.Assert(obtained, gc.Equals, expected)
}

type mockPoolListAPI struct {
	attrs map[string]interface{}
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
	return params.StoragePool{
		Name:     aname,
		Provider: atype,
		Attrs:    s.attrs,
	}
}
