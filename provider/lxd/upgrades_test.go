// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/provider/lxd"
)

type upgradesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&upgradesSuite{})

func (s *upgradesSuite) TestReadLegacyCloudCredentials(c *gc.C) {
	var paths []string
	readFile := func(path string) ([]byte, error) {
		paths = append(paths, path)
		return []byte("content: " + path), nil
	}
	cred, err := lxd.ReadLegacyCloudCredentials(readFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cred, jc.DeepEquals, cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": "content: /etc/juju/lxd-client.crt",
		"client-key":  "content: /etc/juju/lxd-client.key",
		"server-cert": "content: /etc/juju/lxd-server.crt",
	}))
	c.Assert(paths, jc.DeepEquals, []string{
		"/etc/juju/lxd-client.crt",
		"/etc/juju/lxd-client.key",
		"/etc/juju/lxd-server.crt",
	})
}

func (s *upgradesSuite) TestReadLegacyCloudCredentialsFileNotExist(c *gc.C) {
	readFile := func(path string) ([]byte, error) {
		return nil, os.ErrNotExist
	}
	_, err := lxd.ReadLegacyCloudCredentials(readFile)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
