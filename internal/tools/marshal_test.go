// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"github.com/juju/mgo/v3/bson"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/tools"
)

var _ = gc.Suite(&marshalSuite{})

type marshalSuite struct {
}

func newTools(vers, url string) *tools.Tools {
	return &tools.Tools{
		Version: semversion.MustParseBinary(vers),
		URL:     url,
		Size:    10,
		SHA256:  "1234",
	}
}

func (s *marshalSuite) TestMarshalUnmarshal(c *gc.C) {
	c.Skip("we don't want to continue to support bson serialization for versions")
	testTools := newTools("7.8.9-ubuntu-amd64", "http://arble.tgz")
	data, err := bson.Marshal(&testTools)
	c.Assert(err, jc.ErrorIsNil)

	// Check the exact document.
	want := bson.M{
		"version": testTools.Version.String(),
		"url":     testTools.URL,
		"size":    testTools.Size,
		"sha256":  testTools.SHA256,
	}
	got := bson.M{}
	err = bson.Unmarshal(data, &got)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, want)

	// Check that it unpacks properly too.
	var t tools.Tools
	err = bson.Unmarshal(data, &t)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t, gc.Equals, *testTools)
}

func (s *marshalSuite) TestUnmarshalNilRoundtrip(c *gc.C) {
	// We have a custom unmarshaller that should keep
	// the field unset when it finds a nil value.
	var v struct{ Tools *tools.Tools }
	data, err := bson.Marshal(&v)
	c.Assert(err, jc.ErrorIsNil)
	err = bson.Unmarshal(data, &v)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v.Tools, gc.IsNil)
}
