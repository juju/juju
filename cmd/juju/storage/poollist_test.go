// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/juju/tc"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/rpc/params"
)

type poolListSuite struct {
	SubStorageSuite
	mockAPI *mockPoolListAPI
}

var _ = tc.Suite(&poolListSuite{})

func (s *poolListSuite) SetUpTest(c *tc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockPoolListAPI{
		attrs: map[string]interface{}{"key": "value", "one": "1", "two": 2},
	}
}

func (s *poolListSuite) runPoolList(c *tc.C, args []string) (*cmd.Context, error) {
	args = append(args, []string{"-m", "controller"}...)
	return cmdtesting.RunCommand(c, storage.NewPoolListCommandForTest(s.mockAPI, s.store), args...)
}

func (s *poolListSuite) TestPoolListEmpty(c *tc.C) {
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

const (
	providerA = "a"
	providerB = "b"

	nameABC = "abc"
	nameXYZ = "xyz"
)

func (s *poolListSuite) TestPoolListYAML(c *tc.C) {
	s.assertUnmarshalledOutput(c, goyaml.Unmarshal,
		"--provider", providerA,
		"--provider", providerB,
		"--name", nameABC,
		"--name", nameXYZ,
		"--format", "yaml")
}

func (s *poolListSuite) TestPoolListJSON(c *tc.C) {
	s.assertUnmarshalledOutput(c, json.Unmarshal,
		"--provider", providerA,
		"--provider", providerB,
		"--name", nameABC,
		"--name", nameXYZ,
		"--format", "json")
}

func (s *poolListSuite) TestPoolListTabular(c *tc.C) {
	s.assertValidList(
		c,
		[]string{"--provider", "a", "--provider", "b",
			"--name", "xyz", "--name", "abc",
			"--format", "tabular"},
		`
Name       Provider  Attributes
abc        testType  key=value one=1 two=2
testName0  a         key=value one=1 two=2
testName1  b         key=value one=1 two=2
xyz        testType  key=value one=1 two=2
`[1:])
}

func (s *poolListSuite) TestPoolListTabularSortedWithAttrs(c *tc.C) {
	s.mockAPI.attrs = map[string]interface{}{
		"a": true, "c": "well", "b": "maybe"}

	s.assertValidList(
		c,
		[]string{"--name", "myaw", "--name", "xyz", "--name", "abc",
			"--format", "tabular"},
		`
Name  Provider  Attributes
abc   testType  a=true b=maybe c=well
myaw  testType  a=true b=maybe c=well
xyz   testType  a=true b=maybe c=well
`[1:])
}

type unmarshaller func(in []byte, out interface{}) (err error)

func (s *poolListSuite) assertUnmarshalledOutput(c *tc.C, unmarshall unmarshaller, args ...string) {

	context, err := s.runPoolList(c, args)
	c.Assert(err, tc.ErrorIsNil)
	var result map[string]storage.PoolInfo
	err = unmarshall(context.Stdout.(*bytes.Buffer).Bytes(), &result)
	c.Assert(err, tc.ErrorIsNil)
	expected := s.expect(c,
		[]string{providerA, providerB},
		[]string{nameABC, nameXYZ})
	// This comparison cannot rely on gc.DeepEquals as
	// json.Unmarshal unmarshalls the number as a float64,
	// rather than an int
	s.assertSamePoolInfos(c, result, expected)
}

func (s poolListSuite) assertSamePoolInfos(c *tc.C, one, two map[string]storage.PoolInfo) {
	c.Assert(one, tc.HasLen, len(two))

	sameAttributes := func(a, b map[string]interface{}) {
		c.Assert(a, tc.HasLen, len(b))
		for ka, va := range a {
			vb, okb := b[ka]
			c.Assert(okb, tc.IsTrue)
			// As some types may have been unmarshalled incorrectly, for example
			// int versus float64, compare values' string representations
			c.Assert(fmt.Sprintf("%v", va), tc.DeepEquals, fmt.Sprintf("%v", vb))
		}
	}

	for key, v1 := range one {
		v2, ok := two[key]
		c.Assert(ok, tc.IsTrue)
		c.Assert(v1.Provider, tc.Equals, v2.Provider)
		sameAttributes(v1.Attrs, v2.Attrs)
	}
}

func (s poolListSuite) expect(c *tc.C, types, names []string) map[string]storage.PoolInfo {
	all, err := s.mockAPI.ListPools(context.Background(), types, names)
	c.Assert(err, tc.ErrorIsNil)
	result := make(map[string]storage.PoolInfo, len(all))
	for _, one := range all {
		result[one.Name] = storage.PoolInfo{one.Provider, one.Attrs}
	}
	return result
}

func (s *poolListSuite) assertValidList(c *tc.C, args []string, expected string) {
	context, err := s.runPoolList(c, args)
	c.Assert(err, tc.ErrorIsNil)

	obtained := cmdtesting.Stdout(context)
	c.Assert(obtained, tc.Equals, expected)
}

type mockPoolListAPI struct {
	attrs map[string]interface{}
}

func (s mockPoolListAPI) Close() error {
	return nil
}

func (s mockPoolListAPI) ListPools(ctx context.Context, types []string, names []string) ([]params.StoragePool, error) {
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
