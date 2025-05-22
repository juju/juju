// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"bytes"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/testing"
)

type versionSuite struct {
	testing.BaseSuite
}

func TestVersionSuite(t *stdtesting.T) {
	tc.Run(t, &versionSuite{})
}

func getVersions(c *tc.C) *tools.Versions {
	r := bytes.NewReader([]byte(data))
	versions, err := tools.ParseVersions(r)
	c.Assert(err, tc.ErrorIsNil)
	return versions
}

func (s *versionSuite) TestParseVersions(c *tc.C) {
	c.Assert(getVersions(c), tc.DeepEquals, &tools.Versions{
		Versions: []tools.VersionHash{{
			Version: "2.2.4-ubuntu-amd64",
			SHA256:  "eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c",
		}, {
			Version: "2.2.4-windows-amd64",
			SHA256:  "eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c",
		}, {
			Version: "2.2.4-centos-amd64",
			SHA256:  "eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c",
		}, {
			Version: "2.2.4-ubuntu-arm64",
			SHA256:  "f6cf381fc20545d827b307dd413377bff3c123e1894fdf6c239f07a4143beb47",
		}},
	})
}

func (s *versionSuite) TestVersionsMatching(c *tc.C) {
	v := getVersions(c)
	results, err := v.VersionsMatching(bytes.NewReader([]byte(fakeContent1)))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []string{
		"2.2.4-ubuntu-amd64",
		"2.2.4-windows-amd64",
		"2.2.4-centos-amd64",
	})
	results, err = v.VersionsMatching(bytes.NewReader([]byte(fakeContent2)))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, []string{
		"2.2.4-ubuntu-arm64",
	})
}

func (s *versionSuite) TestVersionsMatchingHash(c *tc.C) {
	v := getVersions(c)
	results := tools.VersionsMatchingHash(v,
		"eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c")
	c.Assert(results, tc.DeepEquals, []string{
		"2.2.4-ubuntu-amd64",
		"2.2.4-windows-amd64",
		"2.2.4-centos-amd64",
	})
	results = tools.VersionsMatchingHash(v,
		"f6cf381fc20545d827b307dd413377bff3c123e1894fdf6c239f07a4143beb47")
	c.Assert(results, tc.DeepEquals, []string{
		"2.2.4-ubuntu-arm64",
	})
}

const (
	data = `
versions:
  - version: 2.2.4-ubuntu-amd64
    sha256: eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c
  - version: 2.2.4-windows-amd64
    sha256: eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c
  - version: 2.2.4-centos-amd64
    sha256: eeead9934c597c7678e989e7fd20bf57056a52ce8e25ace371a83711ad484d0c
  - version: 2.2.4-ubuntu-arm64
    sha256: f6cf381fc20545d827b307dd413377bff3c123e1894fdf6c239f07a4143beb47
`

	fakeContent1 = "fake binary content 1\n"
	fakeContent2 = "fake binary content 2\n"
)
