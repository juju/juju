// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

type ResolvedSuite struct {
	testing.IsolationSuite

	mockAPI *mockResolveAPI
}

func (s *ResolvedSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mockAPI = &mockResolveAPI{Stub: &testing.Stub{}}
}

var _ = gc.Suite(&ResolvedSuite{})

func (s *ResolvedSuite) runResolved(c *gc.C, args []string) error {
	store := jujuclienttesting.MinimalStore()
	cmd := application.NewResolvedCommandForTest(s.mockAPI, store)
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	return err
}

var resolvedTests = []struct {
	args  []string
	err   string
	retry bool
	all   bool
	units []string
}{
	{
		err: `no unit specified`,
	}, {
		args: []string{"jeremy-fisher"},
		err:  `unit name "jeremy-fisher" not valid`,
	}, {
		args: []string{"jeremy-fisher/99", "--all"},
		err:  `specifying unit names\(s\) with --all not supported`,
	}, {
		args: []string{"--all", "--no-retry"},
		all:  true,
	}, {
		args:  []string{"jeremy-fisher/98", "jeremy-fisher/99", "--no-retry"},
		units: []string{"jeremy-fisher/98", "jeremy-fisher/99"},
	}, {
		args:  []string{"jeremy-fisher/98", "jeremy-fisher/99"},
		units: []string{"jeremy-fisher/98", "jeremy-fisher/99"},
		retry: true,
	},
}

func (s *ResolvedSuite) TestResolved(c *gc.C) {
	for i, t := range resolvedTests {
		s.mockAPI.ResetCalls()
		c.Logf("test %d: %v", i, t.args)
		err := s.runResolved(c, t.args)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
		s.mockAPI.CheckCallNames(c, "ResolveUnitErrors", "Close")
		s.mockAPI.CheckCall(c, 0, "ResolveUnitErrors", t.units, t.all, t.retry)
	}
}

type mockResolveAPI struct {
	*testing.Stub
	addRelationFunc func(endpoints, viaCIDRs []string) (*params.AddRelationResults, error)
}

func (s mockResolveAPI) Close() error {
	s.MethodCall(s, "Close")
	return s.NextErr()
}

func (s mockResolveAPI) ResolveUnitErrors(units []string, all, retry bool) error {
	s.MethodCall(s, "ResolveUnitErrors", units, all, retry)
	return nil
}
