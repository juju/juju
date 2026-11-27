// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshinit_test

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/internal/cloudconfig/sshinit"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type sshInitSuite struct {
	coretesting.BaseSuite
}

func TestSshInitSuite(t *testing.T) {
	tc.Run(t, &sshInitSuite{})
}

// fakeSSH runs the remote shell commands against a local staging directory.
func (s *sshInitSuite) fakeSSH(c *tc.C, root, host, mode string) ssh.Client {
	script := fmt.Sprintf(`#!/bin/bash --norc
set -euo pipefail
found=0
args=()
for arg in "$@"; do
    if [[ $found == 1 ]]; then
        args+=("$arg")
    elif [[ "$arg" == %q ]]; then
        found=1
    fi
done
[[ $found == 1 ]]
if [[ ${args[0]} == umask && %q == fail ]]; then
    echo 'remote write failed' >&2
    exit 1
fi
if [[ ${args[0]} == sudo && %q == fail-setup ]]; then
    exit 7
fi
command="${args[*]}"
command="${command//\/tmp\//%s/}"
eval "$command"
`, host, mode, mode, root+"/")
	testhelpers.PatchExecutable(c, s, "ssh", script)
	client, err := ssh.NewOpenSSHClient()
	c.Assert(err, tc.ErrorIsNil)
	return client
}

func stagedLocalPath(root, remote string) string {
	return filepath.Join(root, strings.TrimPrefix(remote, "/tmp/"))
}

func (s *sshInitSuite) TestTransfersBytesAndFilePrivately(c *tc.C) {
	root := c.MkDir()
	const host = "ubuntu@2001:db8::1"
	client := s.fakeSSH(c, root, host, "")
	ft, err := sshinit.NewFileTransporter(sshinit.ConfigureParams{Client: client, Host: host})
	c.Assert(err, tc.ErrorIsNil)
	bytesPath := ft.SendBytes("/var/lib/juju/secret archive", []byte("archive bytes"))
	source := filepath.Join(c.MkDir(), "agent.tgz")
	c.Assert(os.WriteFile(source, []byte("agent bytes"), 0600), tc.ErrorIsNil)
	filePath := ft.SendFile("/var/lib/juju/agent.tgz", source)

	c.Assert(ft.Dispatch(c.Context()), tc.ErrorIsNil)
	for _, test := range []struct{ remote, contents string }{
		{bytesPath, "archive bytes"}, {filePath, "agent bytes"},
	} {
		local := stagedLocalPath(root, test.remote)
		contents, err := os.ReadFile(local)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(string(contents), tc.Equals, test.contents)
		info, err := os.Stat(local)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(info.Mode().Perm(), tc.Equals, os.FileMode(0600))
	}
	dir := filepath.Dir(stagedLocalPath(root, bytesPath))
	info, err := os.Stat(dir)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Mode().Perm(), tc.Equals, os.FileMode(0700))

	c.Assert(ft.Cleanup(c.Context()), tc.ErrorIsNil)
	_, err = os.Stat(dir)
	c.Check(os.IsNotExist(err), tc.IsTrue)
}

func (s *sshInitSuite) TestTransferErrorIncludesDiagnostics(c *tc.C) {
	root := c.MkDir()
	const host = "ubuntu@10.0.0.1"
	client := s.fakeSSH(c, root, host, "fail")
	ft, err := sshinit.NewFileTransporter(sshinit.ConfigureParams{Client: client, Host: host})
	c.Assert(err, tc.ErrorIsNil)
	remote := ft.SendBytes("archive", []byte("payload"))
	err = ft.Dispatch(c.Context())
	c.Check(err, tc.ErrorMatches, `transferring .*: remote write failed: .*`)
	c.Assert(ft.Cleanup(c.Context()), tc.ErrorIsNil)
	_, err = os.Stat(filepath.Dir(stagedLocalPath(root, remote)))
	c.Check(os.IsNotExist(err), tc.IsTrue)
}

func (s *sshInitSuite) TestConfigurationPreservesRemoteExitStatus(c *tc.C) {
	const host = "ubuntu@10.0.0.1"
	client := s.fakeSSH(c, c.MkDir(), host, "fail-setup")
	err := sshinit.RunConfigureScript("exit 7", sshinit.ConfigureParams{
		Client: client, Host: host,
	})
	c.Assert(utils.IsRcPassthroughError(err), tc.IsTrue)
	c.Check(err.(*utils.RcPassthroughError).Code, tc.Equals, 7)
}

func (s *sshInitSuite) TestDoesNotRemovePreexistingDirectory(c *tc.C) {
	root := c.MkDir()
	const host = "ubuntu@10.0.0.1"
	client := s.fakeSSH(c, root, host, "")
	ft, err := sshinit.NewFileTransporter(sshinit.ConfigureParams{Client: client, Host: host})
	c.Assert(err, tc.ErrorIsNil)
	remote := ft.SendBytes("archive", []byte("payload"))
	dir := filepath.Dir(stagedLocalPath(root, remote))
	c.Assert(os.Mkdir(dir, 0700), tc.ErrorIsNil)
	marker := filepath.Join(dir, "existing")
	c.Assert(os.WriteFile(marker, []byte("leave me"), 0600), tc.ErrorIsNil)
	c.Check(ft.Dispatch(c.Context()), tc.ErrorMatches, `creating remote staging directory: .*`)
	c.Assert(ft.Cleanup(c.Context()), tc.ErrorIsNil)
	contents, err := os.ReadFile(marker)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Equals, "leave me")
}

func (s *sshInitSuite) TestUniquePathsForRepeatedHints(c *tc.C) {
	root := c.MkDir()
	const host = "ubuntu@10.0.0.1"
	client := s.fakeSSH(c, root, host, "")
	ft, err := sshinit.NewFileTransporter(sshinit.ConfigureParams{Client: client, Host: host})
	c.Assert(err, tc.ErrorIsNil)
	first := ft.SendBytes("same name", []byte("first"))
	second := ft.SendBytes("same name", []byte("second"))
	c.Check(first == second, tc.IsFalse)
	c.Check(path.Dir(first), tc.Equals, path.Dir(second))
	c.Assert(ft.Dispatch(c.Context()), tc.ErrorIsNil)
	c.Assert(ft.Cleanup(c.Context()), tc.ErrorIsNil)
}

func (s *sshInitSuite) TestTransfersSeveralFiles(c *tc.C) {
	root := c.MkDir()
	const host = "ubuntu@10.0.0.1"
	client := s.fakeSSH(c, root, host, "")
	ft, err := sshinit.NewFileTransporter(sshinit.ConfigureParams{Client: client, Host: host})
	c.Assert(err, tc.ErrorIsNil)
	paths := make(map[string]string)
	for i := range 12 {
		contents := fmt.Sprintf("file-%d", i)
		paths[ft.SendBytes(contents, []byte(contents))] = contents
	}
	c.Assert(ft.Dispatch(c.Context()), tc.ErrorIsNil)
	for remote, want := range paths {
		data, err := os.ReadFile(stagedLocalPath(root, remote))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(string(data), tc.Equals, want)
	}
	c.Assert(ft.Cleanup(c.Context()), tc.ErrorIsNil)
}

func (s *sshInitSuite) TestRepeatedDispatch(c *tc.C) {
	root := c.MkDir()
	const host = "ubuntu@10.0.0.1"
	client := s.fakeSSH(c, root, host, "")
	ft, err := sshinit.NewFileTransporter(sshinit.ConfigureParams{Client: client, Host: host})
	c.Assert(err, tc.ErrorIsNil)
	first := ft.SendBytes("first", []byte("first"))
	c.Assert(ft.Dispatch(c.Context()), tc.ErrorIsNil)
	second := ft.SendBytes("second", []byte("second"))
	c.Assert(ft.Dispatch(c.Context()), tc.ErrorIsNil)
	for remote, want := range map[string]string{first: "first", second: "second"} {
		contents, err := os.ReadFile(stagedLocalPath(root, remote))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(string(contents), tc.Equals, want)
	}
	c.Assert(ft.Cleanup(c.Context()), tc.ErrorIsNil)
}
