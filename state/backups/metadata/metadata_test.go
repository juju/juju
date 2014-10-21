// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metadata_test

import (
	"bytes"
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/testing"
)

type metadataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&metadataSuite{}) // Register the suite.

func (s *metadataSuite) TestAsJSONBuffer(c *gc.C) {
	origin := metadata.NewOrigin("asdf-zxcv-qwe", "0", "myhost")
	started := time.Date(2014, time.Month(9), 9, 11, 59, 34, 0, time.UTC)
	finished := started.Add(time.Minute)
	meta := metadata.NewMetadata(*origin, "", &started)
	meta.SetID("20140909-115934.asdf-zxcv-qwe")
	err := meta.Finish(10, "123af2cef", "my hash", &finished)
	c.Assert(err, gc.IsNil)

	buf, err := meta.AsJSONBuffer()
	c.Assert(err, gc.IsNil)

	c.Check(buf.(*bytes.Buffer).String(), gc.Equals,
		`{"ID":"20140909-115934.asdf-zxcv-qwe","Started":"2014-09-09T11:59:34Z","Finished":"2014-09-09T12:00:34Z","Checksum":"123af2cef","ChecksumFormat":"my hash","Size":10,"Stored":false,"Notes":"","Environment":"asdf-zxcv-qwe","Machine":"0","Hostname":"myhost","Version":"1.21-alpha2"}`)
}
