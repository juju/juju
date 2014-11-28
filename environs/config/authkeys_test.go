// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/utils/ssh"
)

type AuthKeysSuite struct {
	testing.BaseSuite
	dotssh string // ~/.ssh
}

var _ = gc.Suite(&AuthKeysSuite{})

func (s *AuthKeysSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	old := utils.Home()
	newhome := c.MkDir()
	utils.SetHome(newhome)
	s.AddCleanup(func(*gc.C) { utils.SetHome(old) })
	s.dotssh = filepath.Join(newhome, ".ssh")
	err := os.Mkdir(s.dotssh, 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AuthKeysSuite) TearDownTest(c *gc.C) {
	ssh.ClearClientKeys()
	s.BaseSuite.TearDownTest(c)
}

func (s *AuthKeysSuite) TestReadAuthorizedKeysErrors(c *gc.C) {
	_, err := config.ReadAuthorizedKeys("")
	c.Assert(err, gc.ErrorMatches, "no public ssh keys found")
	_, err = config.ReadAuthorizedKeys(filepath.Join(s.dotssh, "notthere.pub"))
	c.Assert(err, gc.ErrorMatches, "no public ssh keys found")
}

func writeFile(c *gc.C, filename string, contents string) {
	err := ioutil.WriteFile(filename, []byte(contents), 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AuthKeysSuite) TestReadAuthorizedKeys(c *gc.C) {
	writeFile(c, filepath.Join(s.dotssh, "id_rsa.pub"), "id_rsa")
	writeFile(c, filepath.Join(s.dotssh, "identity.pub"), "identity")
	writeFile(c, filepath.Join(s.dotssh, "test.pub"), "test")
	keys, err := config.ReadAuthorizedKeys("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, gc.Equals, "id_rsa\nidentity\n")
	keys, err = config.ReadAuthorizedKeys("test.pub") // relative to ~/.ssh
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, gc.Equals, "test\n")
}

func (s *AuthKeysSuite) TestReadAuthorizedKeysClientKeys(c *gc.C) {
	keydir := filepath.Join(s.dotssh, "juju")
	err := ssh.LoadClientKeys(keydir) // auto-generates a key pair
	c.Assert(err, jc.ErrorIsNil)
	pubkeyFiles := ssh.PublicKeyFiles()
	c.Assert(pubkeyFiles, gc.HasLen, 1)
	data, err := ioutil.ReadFile(pubkeyFiles[0])
	c.Assert(err, jc.ErrorIsNil)
	prefix := strings.Trim(string(data), "\n") + "\n"

	writeFile(c, filepath.Join(s.dotssh, "id_rsa.pub"), "id_rsa")
	writeFile(c, filepath.Join(s.dotssh, "test.pub"), "test")
	keys, err := config.ReadAuthorizedKeys("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, gc.Equals, prefix+"id_rsa\n")
	keys, err = config.ReadAuthorizedKeys("test.pub")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, gc.Equals, prefix+"test\n")
	keys, err = config.ReadAuthorizedKeys("notthere.pub")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, gc.Equals, prefix)
}

func (s *AuthKeysSuite) TestConcatAuthKeys(c *gc.C) {
	for _, test := range []struct{ a, b, result string }{
		{"a", "", "a"},
		{"", "b", "b"},
		{"a", "b", "a\nb"},
		{"a\n", "b", "a\nb"},
	} {
		c.Check(config.ConcatAuthKeys(test.a, test.b), gc.Equals, test.result)
	}
}
