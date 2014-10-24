// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metadata_test

import (
	"bytes"
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type metadataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&metadataSuite{}) // Register the suite.

func (s *metadataSuite) TestAsJSONBuffer(c *gc.C) {
	origin := metadata.Origin{
		Environment: "asdf-zxcv-qwe",
		Machine:     "0",
		Hostname:    "myhost",
		Version:     version.MustParse("1.21-alpha3"),
	}
	started := time.Date(2014, time.Month(9), 9, 11, 59, 34, 0, time.UTC)
	meta := metadata.NewMetadata(origin, "", &started)

	meta.SetID("20140909-115934.asdf-zxcv-qwe")
	err := meta.Finish(10, "123af2cef")
	c.Assert(err, gc.IsNil)

	finished := started.Add(time.Minute)
	meta.Finished = &finished

	buf, err := meta.AsJSONBuffer()
	c.Assert(err, gc.IsNil)

	c.Check(buf.(*bytes.Buffer).String(), gc.Equals, `{`+
		`"ID":"20140909-115934.asdf-zxcv-qwe",`+
		`"Checksum":"123af2cef",`+
		`"ChecksumFormat":"SHA-1, base64 encoded",`+
		`"Size":10,`+
		`"Stored":"0001-01-01T00:00:00Z",`+
		`"Started":"2014-09-09T11:59:34Z",`+
		`"Finished":"2014-09-09T12:00:34Z",`+
		`"Notes":"",`+
		`"Environment":"asdf-zxcv-qwe",`+
		`"Machine":"0",`+
		`"Hostname":"myhost",`+
		`"Version":"1.21-alpha3"`+
		`}`+"\n")
}
