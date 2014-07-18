// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"net/http"

	gc "launchpad.net/gocheck"

	backup "github.com/juju/juju/state/backup/api"
)

//---------------------------
// ParseDigestHeader()

func (b *BackupSuite) TestParseDigestHeader(c *gc.C) {
	header := http.Header{}
	header.Add("digest", "SHA=<some SHA-1 digest>")
	digests, err := backup.ParseDigestHeader(header)
	c.Check(err, gc.IsNil)

	c.Check(digests, gc.HasLen, 1)
	c.Check(digests["SHA"], gc.Equals, "<some SHA-1 digest>")
}

func (b *BackupSuite) TestParseDigestHeaderMultiple(c *gc.C) {
	header := http.Header{}
	header.Add("digest", "SHA=<some SHA-1 digest>,MD5=<some MD5 digest>")
	digests, err := backup.ParseDigestHeader(header)
	c.Check(err, gc.IsNil)

	c.Check(digests, gc.HasLen, 2)
	c.Check(digests["SHA"], gc.Equals, "<some SHA-1 digest>")
	c.Check(digests["MD5"], gc.Equals, "<some MD5 digest>")
}

func (b *BackupSuite) TestParseDigestHeaderMissing(c *gc.C) {
	header := http.Header{}
	_, err := backup.ParseDigestHeader(header)
	c.Check(err, gc.ErrorMatches, `missing or blank "digest" header`)
}

func (b *BackupSuite) TestParseDigestHeaderEmpty(c *gc.C) {
	header := http.Header{}
	header.Add("digest", "")
	_, err := backup.ParseDigestHeader(header)
	c.Check(err, gc.ErrorMatches, `missing or blank "digest" header`)
}

func (b *BackupSuite) TestParseDigestHeaderMalformed(c *gc.C) {
	header := http.Header{}
	header.Add("digest", "SHA<some SHA-1 digest>")
	_, err := backup.ParseDigestHeader(header)
	c.Check(err, gc.ErrorMatches, `bad "digest" header: .*`)
}

func (b *BackupSuite) TestParseDigestHeaderNoAlgorithm(c *gc.C) {
	header := http.Header{}
	header.Add("digest", "=<some digest>")
	_, err := backup.ParseDigestHeader(header)
	c.Check(err, gc.ErrorMatches, `missing digest algorithm: .*`)
}

func (b *BackupSuite) TestParseDigestHeaderNoValue(c *gc.C) {
	header := http.Header{}
	header.Add("digest", "SHA=")
	_, err := backup.ParseDigestHeader(header)
	c.Check(err, gc.ErrorMatches, `missing digest value: .*`)
}

func (b *BackupSuite) TestParseDigestHeaderDuplicate(c *gc.C) {
	header := http.Header{}
	header.Add("digest", "SHA=<a SHA-1 digest>,SHA=<another SHA-1 digest>")
	_, err := backup.ParseDigestHeader(header)
	c.Check(err, gc.ErrorMatches, "duplicate digest: .*")
}

//---------------------------
// ParseDigest()

func (b *BackupSuite) TestParseDigest(c *gc.C) {
	header := http.Header{}
	header.Add("digest", "SHA=<some SHA-1 digest>")
	digest, err := backup.ParseDigest(header)
	c.Check(err, gc.IsNil)

	c.Check(digest, gc.Equals, "<some SHA-1 digest>")
}

func (b *BackupSuite) TestParseDigestMultiple(c *gc.C) {
	header := http.Header{}
	header.Add("digest", "SHA=<some SHA-1 digest>,MD5=<some MD5 digest>")
	digest, err := backup.ParseDigest(header)
	c.Check(err, gc.IsNil)

	c.Check(digest, gc.Equals, "<some SHA-1 digest>")
}

func (b *BackupSuite) TestParseDigestMissing(c *gc.C) {
	header := http.Header{}
	_, err := backup.ParseDigest(header)
	c.Check(err, gc.ErrorMatches, `missing or blank "digest" header`)
}

func (b *BackupSuite) TestParseDigestNoSHA(c *gc.C) {
	header := http.Header{}
	header.Add("digest", "MD5=<some MD5 digest>")
	_, err := backup.ParseDigest(header)
	c.Check(err, gc.ErrorMatches, `"SHA" missing from "digest" header`)
}
