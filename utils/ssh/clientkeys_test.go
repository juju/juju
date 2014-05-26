// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"io/ioutil"
	"os"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/ssh"
)

type ClientKeysSuite struct {
	testing.FakeHomeSuite
}

var _ = gc.Suite(&ClientKeysSuite{})

func (s *ClientKeysSuite) SetUpTest(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)
	s.AddCleanup(func(*gc.C) { ssh.ClearClientKeys() })
	generateKeyRestorer := overrideGenerateKey(c)
	s.AddCleanup(func(*gc.C) { generateKeyRestorer.Restore() })
}

func checkFiles(c *gc.C, obtained, expected []string) {
	var err error
	for i, e := range expected {
		expected[i], err = utils.NormalizePath(e)
		c.Assert(err, gc.IsNil)
	}
	c.Assert(obtained, jc.SameContents, expected)
}

func checkPublicKeyFiles(c *gc.C, expected ...string) {
	keys := ssh.PublicKeyFiles()
	checkFiles(c, keys, expected)
}

func checkPrivateKeyFiles(c *gc.C, expected ...string) {
	keys := ssh.PrivateKeyFiles()
	checkFiles(c, keys, expected)
}

func (s *ClientKeysSuite) TestPublicKeyFiles(c *gc.C) {
	// LoadClientKeys will create the specified directory
	// and populate it with a key pair.
	err := ssh.LoadClientKeys("~/.juju/ssh")
	c.Assert(err, gc.IsNil)
	checkPublicKeyFiles(c, "~/.juju/ssh/juju_id_rsa.pub")
	// All files ending with .pub in the client key dir get picked up.
	priv, pub, err := ssh.GenerateKey("whatever")
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(testing.HomePath(".juju", "ssh", "whatever.pub"), []byte(pub), 0600)
	c.Assert(err, gc.IsNil)
	err = ssh.LoadClientKeys("~/.juju/ssh")
	c.Assert(err, gc.IsNil)
	// The new public key won't be observed until the
	// corresponding private key exists.
	checkPublicKeyFiles(c, "~/.juju/ssh/juju_id_rsa.pub")
	err = ioutil.WriteFile(testing.HomePath(".juju", "ssh", "whatever"), []byte(priv), 0600)
	c.Assert(err, gc.IsNil)
	err = ssh.LoadClientKeys("~/.juju/ssh")
	c.Assert(err, gc.IsNil)
	checkPublicKeyFiles(c, "~/.juju/ssh/juju_id_rsa.pub", "~/.juju/ssh/whatever.pub")
}

func (s *ClientKeysSuite) TestPrivateKeyFiles(c *gc.C) {
	// Create/load client keys. They will be cached in memory:
	// any files added to the directory will not be considered
	// unless LoadClientKeys is called again.
	err := ssh.LoadClientKeys("~/.juju/ssh")
	c.Assert(err, gc.IsNil)
	checkPrivateKeyFiles(c, "~/.juju/ssh/juju_id_rsa")
	priv, pub, err := ssh.GenerateKey("whatever")
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(testing.HomePath(".juju", "ssh", "whatever"), []byte(priv), 0600)
	c.Assert(err, gc.IsNil)
	err = ssh.LoadClientKeys("~/.juju/ssh")
	c.Assert(err, gc.IsNil)
	// The new private key won't be observed until the
	// corresponding public key exists.
	checkPrivateKeyFiles(c, "~/.juju/ssh/juju_id_rsa")
	err = ioutil.WriteFile(testing.HomePath(".juju", "ssh", "whatever.pub"), []byte(pub), 0600)
	c.Assert(err, gc.IsNil)
	// new keys won't be reported until we call LoadClientKeys again
	checkPublicKeyFiles(c, "~/.juju/ssh/juju_id_rsa.pub")
	checkPrivateKeyFiles(c, "~/.juju/ssh/juju_id_rsa")
	err = ssh.LoadClientKeys("~/.juju/ssh")
	c.Assert(err, gc.IsNil)
	checkPublicKeyFiles(c, "~/.juju/ssh/juju_id_rsa.pub", "~/.juju/ssh/whatever.pub")
	checkPrivateKeyFiles(c, "~/.juju/ssh/juju_id_rsa", "~/.juju/ssh/whatever")
}

func (s *ClientKeysSuite) TestLoadClientKeysDirExists(c *gc.C) {
	err := os.MkdirAll(testing.HomePath(".juju", "ssh"), 0755)
	c.Assert(err, gc.IsNil)
	err = ssh.LoadClientKeys("~/.juju/ssh")
	c.Assert(err, gc.IsNil)
	checkPrivateKeyFiles(c, "~/.juju/ssh/juju_id_rsa")
}
