// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
)

type AuthKeysSuite struct {
	testing.BaseSuite
	dotssh string // ~/.ssh
}

var _ = tc.Suite(&AuthKeysSuite{})

func (s *AuthKeysSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	old := utils.Home()
	newhome := c.MkDir()
	err := utils.SetHome(newhome)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *tc.C) {
		ssh.ClearClientKeys()
		err := utils.SetHome(old)
		c.Assert(err, jc.ErrorIsNil)
	})

	s.dotssh = filepath.Join(newhome, ".ssh")
	err = os.Mkdir(s.dotssh, 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AuthKeysSuite) TestReadAuthorizedKeysErrors(c *tc.C) {
	ctx := cmdtesting.Context(c)
	_, err := common.ReadAuthorizedKeys(ctx, "")
	c.Assert(err, tc.ErrorMatches, "no public ssh keys found")
	c.Assert(err, tc.Equals, common.ErrNoAuthorizedKeys)
	_, err = common.ReadAuthorizedKeys(ctx, filepath.Join(s.dotssh, "notthere.pub"))
	c.Assert(err, tc.ErrorMatches, "no public ssh keys found")
	c.Assert(err, tc.Equals, common.ErrNoAuthorizedKeys)
}

func writeFile(c *tc.C, filename string, contents string) {
	err := os.WriteFile(filename, []byte(contents), 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *AuthKeysSuite) TestReadAuthorizedKeys(c *tc.C) {
	ctx := cmdtesting.Context(c)
	writeFile(c, filepath.Join(s.dotssh, "id_rsa.pub"), "id_rsa")
	writeFile(c, filepath.Join(s.dotssh, "id_dsa.pub"), "id_dsa") // Check dsa is NOT loaded
	writeFile(c, filepath.Join(s.dotssh, "id_ed25519.pub"), "id_ed25519")
	writeFile(c, filepath.Join(s.dotssh, "identity.pub"), "identity")
	writeFile(c, filepath.Join(s.dotssh, "test.pub"), "test")
	keys, err := common.ReadAuthorizedKeys(ctx, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, tc.Equals, "id_ed25519\nid_rsa\nidentity\n")
	keys, err = common.ReadAuthorizedKeys(ctx, "test.pub") // relative to ~/.ssh
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, tc.Equals, "test\n")
}

func (s *AuthKeysSuite) TestReadAuthorizedKeysClientKeys(c *tc.C) {
	ctx := cmdtesting.Context(c)
	keydir := filepath.Join(s.dotssh, "juju")
	err := ssh.LoadClientKeys(keydir) // auto-generates a key pair
	c.Assert(err, jc.ErrorIsNil)
	pubkeyFiles := ssh.PublicKeyFiles()
	c.Assert(pubkeyFiles, tc.HasLen, 1)
	data, err := os.ReadFile(pubkeyFiles[0])
	c.Assert(err, jc.ErrorIsNil)
	prefix := strings.Trim(string(data), "\n") + "\n"

	writeFile(c, filepath.Join(s.dotssh, "id_rsa.pub"), "id_rsa")
	writeFile(c, filepath.Join(s.dotssh, "test.pub"), "test")
	keys, err := common.ReadAuthorizedKeys(ctx, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, tc.Equals, prefix+"id_rsa\n")
	keys, err = common.ReadAuthorizedKeys(ctx, "test.pub")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, tc.Equals, prefix+"test\n")
	keys, err = common.ReadAuthorizedKeys(ctx, "notthere.pub")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(keys, tc.Equals, prefix)
}
