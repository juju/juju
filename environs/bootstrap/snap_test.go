// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/bootstrap"
	coretesting "github.com/juju/juju/internal/testing"
)

type snapReaderSuite struct {
	coretesting.BaseSuite
}

func TestSnapReaderSuite(t *testing.T) {
	tc.Run(t, &snapReaderSuite{})
}

func (s *snapReaderSuite) setupUnsquashfs(c *tc.C, snapOut string, cmdErr error) {
	origFind := bootstrap.FindUnsquashfs
	restoreFind := func() { *bootstrap.FindUnsquashfs = *origFind }
	c.Cleanup(restoreFind)
	*bootstrap.FindUnsquashfs = func() (string, error) {
		return "/usr/bin/unsquashfs", nil
	}

	origRun := bootstrap.RunUnsquashfsCommand
	restoreRun := func() { *bootstrap.RunUnsquashfsCommand = *origRun }
	c.Cleanup(restoreRun)
	*bootstrap.RunUnsquashfsCommand = func(_ context.Context, _ string, _ string) ([]byte, error) {
		return []byte(snapOut), cmdErr
	}
}

func (s *snapReaderSuite) TestInspectLocalSnapVersionSuccess(c *tc.C) {
	snapOut := `name:    jujud
summary: Juju Controller Daemon
publisher: Canonical
version: 4.1-beta2
contact: https://bugs.launchpad.net/juju
`
	s.setupUnsquashfs(c, snapOut, nil)

	vers, err := bootstrap.InspectLocalSnapVersion(context.Background(), "/path/to/jujud.snap")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vers, tc.DeepEquals, semversion.MustParse("4.1-beta2"))
}

func (s *snapReaderSuite) TestReadSnapVersionReturnsRawVersion(c *tc.C) {
	snapOut := `name: jujud
version: 4.1-beta2-06aa059
`
	s.setupUnsquashfs(c, snapOut, nil)

	raw, vers, err := bootstrap.ReadSnapVersion(context.Background(), "/path/to/jujud.snap")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(raw, tc.Equals, "4.1-beta2-06aa059")
	c.Check(vers, tc.DeepEquals, semversion.MustParse("4.1-beta2"))
}

func (s *snapReaderSuite) TestReadSnapVersionUnsquashfsUnavailable(c *tc.C) {
	origFind := bootstrap.FindUnsquashfs
	restoreFind := func() { *bootstrap.FindUnsquashfs = *origFind }
	defer restoreFind()
	*bootstrap.FindUnsquashfs = func() (string, error) {
		return "", fmt.Errorf("exec: not found")
	}

	_, _, err := bootstrap.ReadSnapVersion(context.Background(), "/path/to/jujud.snap")
	c.Assert(err, tc.ErrorMatches, `.*unsquashfs is required to inspect a controller snap.*`)
}

func (s *snapReaderSuite) TestInspectLocalSnapVersionUnsquashfsFails(c *tc.C) {
	const snapPath = "/path/to/jujud.snap"
	s.setupUnsquashfs(c, "", fmt.Errorf("unsquashfs bombed"))

	_, err := bootstrap.InspectLocalSnapVersion(context.Background(), snapPath)
	c.Assert(err, tc.ErrorMatches, `.*inspecting local snap.*extract meta/snap.yaml.*`)
	c.Check(
		strings.Contains(err.Error(), snapPath), tc.IsTrue,
		tc.Commentf("expected error to mention snap path %q, got: %s", snapPath, err),
	)
}

func (s *snapReaderSuite) TestInspectLocalSnapVersionNoVersionLine(c *tc.C) {
	const snapPath = "/path/to/jujud.snap"
	snapOut := `name:    jujud
summary: Juju Controller Daemon
`

	s.setupUnsquashfs(c, snapOut, nil)

	_, err := bootstrap.InspectLocalSnapVersion(context.Background(), snapPath)
	c.Assert(err, tc.ErrorMatches, `.*no version.*`)
	c.Check(
		strings.Contains(err.Error(), snapPath), tc.IsTrue,
		tc.Commentf("expected error to mention snap path %q, got: %s", snapPath, err),
	)
}

func (s *snapReaderSuite) TestInspectLocalSnapVersionUnparsableVersion(c *tc.C) {
	const snapPath = "/path/to/jujud.snap"
	snapOut := `name:    jujud
version: not-a-version
`

	s.setupUnsquashfs(c, snapOut, nil)

	_, err := bootstrap.InspectLocalSnapVersion(context.Background(), snapPath)
	c.Assert(err, tc.ErrorMatches, `.*cannot parse.*not-a-version.*`)
	c.Check(
		strings.Contains(err.Error(), snapPath), tc.IsTrue,
		tc.Commentf("expected error to mention snap path %q, got: %s", snapPath, err),
	)
}
