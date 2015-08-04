// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/testing"
)

type CommonSuite struct {
	BaseSuite
	serverFilename string
}

var _ = gc.Suite(&CommonSuite{})

func (s *CommonSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.serverFilename = ""
	s.PatchValue(user.ServerFileNotify, func(filename string) {
		s.serverFilename = filename
	})
}

// ConnectionEndpoint so this suite implements the EndpointProvider interface.
func (s *CommonSuite) ConnectionEndpoint() (configstore.APIEndpoint, error) {
	return configstore.APIEndpoint{
		// NOTE: the content here is the same as t
		Addresses: []string{"127.0.0.1:12345"},
		CACert:    testing.CACert,
	}, nil
}

func (s *CommonSuite) TestAbsolutePath(c *gc.C) {
	ctx := testing.Context(c)
	err := user.WriteServerFile(s, ctx, "username", "password", "outfile.blah")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filepath.IsAbs(s.serverFilename), jc.IsTrue)
	c.Assert(s.serverFilename, gc.Equals, filepath.Join(ctx.Dir, "outfile.blah"))
}

func (s *CommonSuite) TestFileContent(c *gc.C) {
	ctx := testing.Context(c)
	err := user.WriteServerFile(s, ctx, "username", "password", "outfile.blah")
	c.Assert(err, jc.ErrorIsNil)
	s.assertServerFileMatches(c, s.serverFilename, "username", "password")
}

func (s *CommonSuite) TestWriteServerFileBadUser(c *gc.C) {
	ctx := testing.Context(c)
	err := user.WriteServerFile(s, ctx, "bad user", "password", "outfile.blah")
	c.Assert(err, gc.ErrorMatches, `"bad user" is not a valid username`)
}
