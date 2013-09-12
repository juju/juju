// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams_test

import (
	"io/ioutil"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/simplestreams/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

var _ = gc.Suite(&datasourceSuite{})

type datasourceSuite struct {
	testing.TestDataSuite
}

func (s *datasourceSuite) TestFetch(c *gc.C) {
	ds := simplestreams.NewURLDataSource("test:")
	rc, url, err := ds.Fetch("streams/v1/tools_metadata.json")
	c.Assert(err, gc.IsNil)
	defer rc.Close()
	c.Assert(url, gc.Equals, "test:/streams/v1/tools_metadata.json")
	data, err := ioutil.ReadAll(rc)
	cloudMetadata, err := simplestreams.ParseCloudMetadata(data, "products:1.0", url, imagemetadata.ImageMetadata{})
	c.Assert(err, gc.IsNil)
	c.Assert(len(cloudMetadata.Products), jc.GreaterThan, 0)
}

func (s *datasourceSuite) TestURL(c *gc.C) {
	ds := simplestreams.NewURLDataSource("foo")
	url, err := ds.URL("bar")
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.Equals, "foo/bar")
}
