// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/internal/provider/lxd"
)

type upgradesSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&upgradesSuite{})

func (s *upgradesSuite) TestReadLegacyCloudCredentials(c *tc.C) {
	var paths []string
	readFile := func(path string) ([]byte, error) {
		paths = append(paths, path)
		return []byte("content: " + path), nil
	}
	cred, err := lxd.ReadLegacyCloudCredentials(readFile)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cred, tc.DeepEquals, cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": "content: /etc/juju/lxd-client.crt",
		"client-key":  "content: /etc/juju/lxd-client.key",
		"server-cert": "content: /etc/juju/lxd-server.crt",
	}))
	c.Assert(paths, tc.DeepEquals, []string{
		"/etc/juju/lxd-client.crt",
		"/etc/juju/lxd-client.key",
		"/etc/juju/lxd-server.crt",
	})
}

func (s *upgradesSuite) TestReadLegacyCloudCredentialsFileNotExist(c *tc.C) {
	readFile := func(path string) ([]byte, error) {
		return nil, os.ErrNotExist
	}
	_, err := lxd.ReadLegacyCloudCredentials(readFile)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}
