// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"bytes"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/testing"
)

type versionSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&versionSuite{})

func getVersions(c *gc.C) *tools.Versions {
	r := bytes.NewReader([]byte(data))
	versions, err := tools.ParseVersions(r)
	c.Assert(err, jc.ErrorIsNil)
	return versions
}

func (s *versionSuite) TestParseVersions(c *gc.C) {
	c.Assert(getVersions(c), gc.DeepEquals, &tools.Versions{
		Versions: []tools.VersionHash{{
			Version: "2.2.4-xenial-amd64",
			SHA256:  "eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c",
		}, {
			Version: "2.2.4-trusty-amd64",
			SHA256:  "eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c",
		}, {
			Version: "2.2.4-centos7-amd64",
			SHA256:  "eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c",
		}, {
			Version: "2.2.4-xenial-arm64",
			SHA256:  "f6cf381fc20545d827b307dd413377bff3c123e1894fdf6c239f07a4143beb47",
		}},
	})
}

func (s *versionSuite) TestVersionsMatching(c *gc.C) {
	v := getVersions(c)
	results, err := v.VersionsMatching(bytes.NewReader([]byte(fakeContent1)))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, []string{
		"2.2.4-xenial-amd64",
		"2.2.4-trusty-amd64",
		"2.2.4-centos7-amd64",
	})
	results, err = v.VersionsMatching(bytes.NewReader([]byte(fakeContent2)))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, []string{
		"2.2.4-xenial-arm64",
	})
}

func (s *versionSuite) TestVersionsMatchingHash(c *gc.C) {
	v := getVersions(c)
	results := tools.VersionsMatchingHash(v,
		"eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c")
	c.Assert(results, gc.DeepEquals, []string{
		"2.2.4-xenial-amd64",
		"2.2.4-trusty-amd64",
		"2.2.4-centos7-amd64",
	})
	results = tools.VersionsMatchingHash(v,
		"f6cf381fc20545d827b307dd413377bff3c123e1894fdf6c239f07a4143beb47")
	c.Assert(results, gc.DeepEquals, []string{
		"2.2.4-xenial-arm64",
	})
}

const (
	data = `
versions:
  - version: 2.2.4-xenial-amd64
    sha256: eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c
  - version: 2.2.4-trusty-amd64
    sha256: eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c
  - version: 2.2.4-centos7-amd64
    sha256: eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c
  - version: 2.2.4-xenial-arm64
    sha256: f6cf381fc20545d827b307dd413377bff3c123e1894fdf6c239f07a4143beb47
`

	fakeContent1 = "fake binary content 1\n"
	fakeContent2 = "fake binary content 2\n"
)
