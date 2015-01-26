// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/charms"
	"github.com/juju/juju/testing"
)

// This suite provides basic tests for the "charms list" command.
type CharmsListCommandSuite struct {
	BaseSuite
	mockAPI *mockListAPI
}

var _ = gc.Suite(&CharmsListCommandSuite{})

func (s *CharmsListCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockListAPI{}
	s.PatchValue(charms.GetCharmsListAPI, func(c *charms.ListCommand) (charms.CharmsListAPI, error) {
		return s.mockAPI, nil
	})
}

func (s *CharmsListCommandSuite) TestListAllCharms(c *gc.C) {
	context, err := testing.RunCommand(c, envcmd.Wrap(&charms.ListCommand{}))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `- charm
- local:charm
- local:charm--1
- local:charm-1
- local:series/charm
- local:series/charm-3
- local:series/charm-0
- cs:~user/charm
- cs:~user/charm-1
- cs:~user/series/charm
- cs:~user/series/charm-1
- cs:series/charm
- cs:series/charm-3
- cs:series/charm-0
- cs:charm
- cs:charm--1
- cs:charm-1
- charm
- charm-1
- series/charm
- series/charm-1
`)
}

func (s *CharmsListCommandSuite) TestListNamedCharm(c *gc.C) {
	fewCharms := []string{"charm"}
	context, err := testing.RunCommand(c, envcmd.Wrap(&charms.ListCommand{}), fewCharms...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `- charm
`)
}

func (s *CharmsListCommandSuite) TestListCharmJSON(c *gc.C) {
	args := []string{"--format", "json", "charm"}
	context, err := testing.RunCommand(c, envcmd.Wrap(&charms.ListCommand{}), args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `["charm"]
`)
}

func (s *CharmsListCommandSuite) TestListError(c *gc.C) {
	s.mockAPI.wantErr = true
	_, err := testing.RunCommand(c, envcmd.Wrap(&charms.ListCommand{}))
	c.Assert(err.Error(), gc.Matches, ".*expected test error.*")
}

type mockListAPI struct {
	wantErr bool
}

func (*mockListAPI) Close() error {
	return nil
}

func (m *mockListAPI) List(names []string) ([]string, error) {
	if m.wantErr {
		return nil, errors.New("expected test error")
	}
	if len(names) > 0 {
		return names, nil
	}
	return []string{"charm",
		"local:charm",
		"local:charm--1",
		"local:charm-1",
		"local:series/charm",
		"local:series/charm-3",
		"local:series/charm-0",
		"cs:~user/charm",
		"cs:~user/charm-1",
		"cs:~user/series/charm",
		"cs:~user/series/charm-1",
		"cs:series/charm",
		"cs:series/charm-3",
		"cs:series/charm-0",
		"cs:charm",
		"cs:charm--1",
		"cs:charm-1",
		"charm",
		"charm-1",
		"series/charm",
		"series/charm-1",
	}, nil
}
