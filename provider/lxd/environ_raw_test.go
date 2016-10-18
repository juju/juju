// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/tools/lxdclient"
)

type environRawSuite struct {
	testing.IsolationSuite
	testing.Stub
	readFile   readFileFunc
	runCommand runCommandFunc
}

var _ = gc.Suite(&environRawSuite{})

func (s *environRawSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub.ResetCalls()
	s.readFile = func(path string) ([]byte, error) {
		s.AddCall("readFile", path)
		if err := s.NextErr(); err != nil {
			return nil, err
		}
		return []byte("content:" + path), nil
	}
	s.runCommand = func(command string, args ...string) (string, error) {
		s.AddCall("runCommand", command, args)
		if err := s.NextErr(); err != nil {
			return "", err
		}
		return "default via 10.0.8.1 dev eth0", nil
	}
}

func (s *environRawSuite) TestGetRemoteConfig(c *gc.C) {
	cfg, err := getRemoteConfig(s.readFile, s.runCommand)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, &lxdclient.Config{
		Remote: lxdclient.Remote{
			Name:     "remote",
			Host:     "10.0.8.1",
			Protocol: "lxd",
			Cert: &lxdclient.Cert{
				CertPEM: []byte("content:/etc/juju/lxd-client.crt"),
				KeyPEM:  []byte("content:/etc/juju/lxd-client.key"),
			},
			ServerPEMCert: "content:/etc/juju/lxd-server.crt",
		},
	})
	s.Stub.CheckCalls(c, []testing.StubCall{
		{"readFile", []interface{}{"/etc/juju/lxd-client.crt"}},
		{"readFile", []interface{}{"/etc/juju/lxd-client.key"}},
		{"readFile", []interface{}{"/etc/juju/lxd-server.crt"}},
		{"runCommand", []interface{}{"ip", []string{"route", "list", "match", "0/0"}}},
	})
}

func (s *environRawSuite) TestGetRemoteConfigFileNotExist(c *gc.C) {
	s.SetErrors(os.ErrNotExist)
	_, err := getRemoteConfig(s.readFile, s.runCommand)
	// os.IsNotExist is translated to errors.IsNotFound
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, "reading client certificate: /etc/juju/lxd-client.crt not found")
}

func (s *environRawSuite) TestGetRemoteConfigFileError(c *gc.C) {
	s.SetErrors(nil, errors.New("i/o error"))
	_, err := getRemoteConfig(s.readFile, s.runCommand)
	c.Assert(err, gc.ErrorMatches, "reading client key: i/o error")
}

func (s *environRawSuite) TestGetRemoteConfigIPRouteFormatError(c *gc.C) {
	s.runCommand = func(string, ...string) (string, error) {
		return "this is not the prefix you're looking for", nil
	}
	_, err := getRemoteConfig(s.readFile, s.runCommand)
	c.Assert(err, gc.ErrorMatches,
		`getting gateway address: unexpected output from "ip route": this is not the prefix you're looking for`)
}

func (s *environRawSuite) TestGetRemoteConfigIPRouteCommandError(c *gc.C) {
	s.SetErrors(nil, nil, nil, errors.New("buh bow"))
	_, err := getRemoteConfig(s.readFile, s.runCommand)
	c.Assert(err, gc.ErrorMatches, `getting gateway address: buh bow`)
}
