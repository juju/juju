// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type sshdConfigSuite struct {
	testhelpers.IsolationSuite
}

func TestSSHDConfigSuite(t *testing.T) {
	tc.Run(t, &sshdConfigSuite{})
}

func (s *sshdConfigSuite) TestParseSSHDConfigPort(c *tc.C) {
	tests := []struct {
		name  string
		input string
		want  string
	}{{
		name:  "default when no port directive",
		input: "# a comment\nPermitRootLogin no\n",
		want:  DefaultSSHDPort,
	}, {
		name:  "explicit port",
		input: "Port 2222\n",
		want:  "2222",
	}, {
		name:  "commented port ignored",
		input: "#Port 99\nPort 2222\n",
		want:  "2222",
	}, {
		name:  "first port wins",
		input: "Port 2222\nPort 3333\n",
		want:  "2222",
	}, {
		name:  "leading whitespace tolerated",
		input: "   Port 2222\n",
		want:  "2222",
	}, {
		name:  "malformed port line falls back to default",
		input: "Port\n",
		want:  DefaultSSHDPort,
	}, {
		name:  "port line with extra fields falls back to default",
		input: "Port 2222 extra\n",
		want:  DefaultSSHDPort,
	}, {
		name:  "empty input",
		input: "",
		want:  DefaultSSHDPort,
	}}

	for _, t := range tests {
		cfg, err := ParseSSHDConfig(strings.NewReader(t.input))
		c.Check(err, tc.ErrorIsNil, tc.Commentf(t.name))
		c.Check(cfg.Port(), tc.Equals, t.want, tc.Commentf(t.name))
	}
}

// errReader is an io.Reader that always fails, used to exercise the read-error
// path of ParseSSHDConfig.
type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.Errorf("boom")
}

func (s *sshdConfigSuite) TestParseSSHDConfigReadError(c *tc.C) {
	_, err := ParseSSHDConfig(errReader{})
	c.Check(err, tc.ErrorMatches, ".*boom.*")
}

func (s *sshdConfigSuite) TestOpenSSHDConfig(c *tc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "sshd_config")
	err := os.WriteFile(path, []byte("Port 2222\n"), 0644)
	c.Assert(err, tc.ErrorIsNil)

	cfg, err := OpenSSHDConfig(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.Port(), tc.Equals, "2222")
}

func (s *sshdConfigSuite) TestOpenSSHDConfigMissingFile(c *tc.C) {
	_, err := OpenSSHDConfig(filepath.Join(c.MkDir(), "does-not-exist"))
	c.Check(err, tc.NotNil)
}
