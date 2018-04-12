// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/testing"
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
	err := utils.SetHome(newhome)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) {
		ssh.ClearClientKeys()
		err := utils.SetHome(old)
		c.Assert(err, jc.ErrorIsNil)
	})

	s.dotssh = filepath.Join(newhome, ".ssh")
	err = os.Mkdir(s.dotssh, 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AuthKeysSuite) TestReadAuthorizedKeysErrors(c *gc.C) {
	ctx := cmdtesting.Context(c)
	_, err := common.ReadAuthorizedKeys(ctx, "")
	c.Assert(err, gc.ErrorMatches, "no public ssh keys found")
	c.Assert(err, gc.Equals, common.ErrNoAuthorizedKeys)
	_, err = common.ReadAuthorizedKeys(ctx, filepath.Join(s.dotssh, "notthere.pub"))
	c.Assert(err, gc.ErrorMatches, "no public ssh keys found")
	c.Assert(err, gc.Equals, common.ErrNoAuthorizedKeys)
}

func writeFile(c *gc.C, filename string, contents string) {
	err := ioutil.WriteFile(filename, []byte(contents), 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AuthKeysSuite) TestReadAuthorizedKeys(c *gc.C) {
	ctx := cmdtesting.Context(c)
	writeFile(c, filepath.Join(s.dotssh, "id_rsa.pub"), "id_rsa")
	writeFile(c, filepath.Join(s.dotssh, "identity.pub"), "identity")
	writeFile(c, filepath.Join(s.dotssh, "test.pub"), "test")
	keys, err := common.ReadAuthorizedKeys(ctx, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, gc.Equals, "id_rsa\nidentity\n")
	keys, err = common.ReadAuthorizedKeys(ctx, "test.pub") // relative to ~/.ssh
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, gc.Equals, "test\n")
}

func (s *AuthKeysSuite) TestReadAuthorizedKeysClientKeys(c *gc.C) {
	ctx := cmdtesting.Context(c)
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
	keys, err := common.ReadAuthorizedKeys(ctx, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, gc.Equals, prefix+"id_rsa\n")
	keys, err = common.ReadAuthorizedKeys(ctx, "test.pub")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, gc.Equals, prefix+"test\n")
	keys, err = common.ReadAuthorizedKeys(ctx, "notthere.pub")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, gc.Equals, prefix)
}

func (s *AuthKeysSuite) TestFinalizeAuthorizedKeysNoop(c *gc.C) {
	attrs := map[string]interface{}{"authorized-keys": "meep"}
	err := common.FinalizeAuthorizedKeys(cmdtesting.Context(c), attrs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attrs, jc.DeepEquals, map[string]interface{}{"authorized-keys": "meep"})
}

func (s *AuthKeysSuite) TestFinalizeAuthorizedKeysPath(c *gc.C) {
	writeFile(c, filepath.Join(s.dotssh, "whatever"), "meep")
	attrs := map[string]interface{}{"authorized-keys-path": "whatever"}
	err := common.FinalizeAuthorizedKeys(cmdtesting.Context(c), attrs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attrs, jc.DeepEquals, map[string]interface{}{"authorized-keys": "meep\n"})
}

func (s *AuthKeysSuite) TestFinalizeAuthorizedKeysDefault(c *gc.C) {
	writeFile(c, filepath.Join(s.dotssh, "id_rsa.pub"), "meep")
	attrs := map[string]interface{}{}
	err := common.FinalizeAuthorizedKeys(cmdtesting.Context(c), attrs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attrs, jc.DeepEquals, map[string]interface{}{"authorized-keys": "meep\n"})
}

func (s *AuthKeysSuite) TestFinalizeAuthorizedKeysConflict(c *gc.C) {
	attrs := map[string]interface{}{"authorized-keys": "foo", "authorized-keys-path": "bar"}
	err := common.FinalizeAuthorizedKeys(cmdtesting.Context(c), attrs)
	c.Assert(err, gc.ErrorMatches, `"authorized-keys" and "authorized-keys-path" may not both be specified`)
}
