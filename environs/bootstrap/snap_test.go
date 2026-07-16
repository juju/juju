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

type snapHelpersSuite struct {
	coretesting.BaseSuite
}

func TestSnapHelpersSuite(t *testing.T) {
	tc.Run(t, &snapHelpersSuite{})
}

func (s *snapHelpersSuite) TestInspectLocalSnapVersionSuccess(c *tc.C) {
	snapOut := `name:    jujud
summary: Juju Controller Daemon
publisher: Canonical
version: 4.1-beta2 -
contact: https://bugs.launchpad.net/juju
`

	orig := bootstrap.RunSnapInfoCommand
	restore := func() { *bootstrap.RunSnapInfoCommand = *orig }
	defer restore()

	*bootstrap.RunSnapInfoCommand = func(_ context.Context, _ string) (string, error) {
		return snapOut, nil
	}

	vers, err := bootstrap.InspectLocalSnapVersion(context.Background(), "/path/to/jujud.snap")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vers, tc.DeepEquals, semversion.MustParse("4.1-beta2"))
}

func (s *snapHelpersSuite) TestInspectLocalSnapVersionSnapInfoFails(c *tc.C) {
	const snapPath = "/path/to/jujud.snap"
	orig := bootstrap.RunSnapInfoCommand
	restore := func() { *bootstrap.RunSnapInfoCommand = *orig }
	defer restore()

	*bootstrap.RunSnapInfoCommand = func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("snap info bombed")
	}

	_, err := bootstrap.InspectLocalSnapVersion(context.Background(), snapPath)
	c.Assert(err, tc.ErrorMatches, `.*inspecting local snap.*snap info.*`)
	c.Check(
		strings.Contains(err.Error(), snapPath), tc.IsTrue,
		tc.Commentf("expected error to mention snap path %q, got: %s", snapPath, err),
	)
}

func (s *snapHelpersSuite) TestInspectLocalSnapVersionNoVersionLine(c *tc.C) {
	const snapPath = "/path/to/jujud.snap"
	snapOut := `name:    jujud
summary: Juju Controller Daemon
`

	orig := bootstrap.RunSnapInfoCommand
	restore := func() { *bootstrap.RunSnapInfoCommand = *orig }
	defer restore()
	*bootstrap.RunSnapInfoCommand = func(_ context.Context, _ string) (string, error) {
		return snapOut, nil
	}

	_, err := bootstrap.InspectLocalSnapVersion(context.Background(), snapPath)
	c.Assert(err, tc.ErrorMatches, `.*no version.*`)
	c.Check(
		strings.Contains(err.Error(), snapPath), tc.IsTrue,
		tc.Commentf("expected error to mention snap path %q, got: %s", snapPath, err),
	)
}

func (s *snapHelpersSuite) TestInspectLocalSnapVersionUnparsableVersion(c *tc.C) {
	const snapPath = "/path/to/jujud.snap"
	snapOut := `name:    jujud
version: not-a-version -
`

	orig := bootstrap.RunSnapInfoCommand
	restore := func() { *bootstrap.RunSnapInfoCommand = *orig }
	defer restore()
	*bootstrap.RunSnapInfoCommand = func(_ context.Context, _ string) (string, error) {
		return snapOut, nil
	}

	_, err := bootstrap.InspectLocalSnapVersion(context.Background(), snapPath)
	c.Assert(err, tc.ErrorMatches, `.*cannot parse.*not-a-version.*`)
	c.Check(
		strings.Contains(err.Error(), snapPath), tc.IsTrue,
		tc.Commentf("expected error to mention snap path %q, got: %s", snapPath, err),
	)
}
