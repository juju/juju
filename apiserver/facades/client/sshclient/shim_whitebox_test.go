// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"github.com/gliderlabs/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type shimSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&shimSuite{})

var (
	hostKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACCP9y2SiMT+bxv25bNA3bpPtNqoZjFVQ5WRQ7/iqsXmRgAAAIiNBL3UjQS9
1AAAAAtzc2gtZWQyNTUxOQAAACCP9y2SiMT+bxv25bNA3bpPtNqoZjFVQ5WRQ7/iqsXmRg
AAAECXJNZYQFl7ccvfCeJPRgqteU7luG7g6lwMOPpPAPCUjo/3LZKIxP5vG/bls0Dduk+0
2qhmMVVDlZFDv+KqxeZGAAAABHRlc3QB
-----END OPENSSH PRIVATE KEY-----
`
)

func (s *shimSuite) TestGetAuthorizedKey(c *gc.C) {
	key, err := getPublicKeyWireFormat([]byte(hostKey))
	c.Assert(err, gc.IsNil)

	_, err = ssh.ParsePublicKey(key)
	c.Assert(err, gc.IsNil)
}
