// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"labix.org/v2/mgo/bson"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

var _ = gc.Suite(&marshalSuite{})

type marshalSuite struct {
}

func newTools(vers, url string) *tools.Tools {
	return &tools.Tools{
		Version: version.MustParseBinary(vers),
		URL:     url,
		Size:    10,
		SHA256:  "1234",
	}
}

func (s *marshalSuite) TestMarshalUnmarshal(c *gc.C) {
	testTools := newTools("7.8.9-foo-bar", "http://arble.tgz")
	data, err := bson.Marshal(&testTools)
	c.Assert(err, gc.IsNil)

	// Check the exact document.
	want := bson.M{
		"version": testTools.Version.String(),
		"url":     testTools.URL,
		"size":    testTools.Size,
		"sha256":  testTools.SHA256,
	}
	got := bson.M{}
	err = bson.Unmarshal(data, &got)
	c.Assert(err, gc.IsNil)
	c.Assert(got, gc.DeepEquals, want)

	// Check that it unpacks properly too.
	var t tools.Tools
	err = bson.Unmarshal(data, &t)
	c.Assert(err, gc.IsNil)
	c.Assert(t, gc.Equals, *testTools)
}

func (s *marshalSuite) TestUnmarshalNilRoundtrip(c *gc.C) {
	// We have a custom unmarshaller that should keep
	// the field unset when it finds a nil value.
	var v struct{ Tools *tools.Tools }
	data, err := bson.Marshal(&v)
	c.Assert(err, gc.IsNil)
	err = bson.Unmarshal(data, &v)
	c.Assert(err, gc.IsNil)
	c.Assert(v.Tools, gc.IsNil)
}
