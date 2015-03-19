// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/storage"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type PoolCreateSuite struct {
	SubStorageSuite
	mockAPI *mockPoolCreateAPI
}

var _ = gc.Suite(&PoolCreateSuite{})

func (s *PoolCreateSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockPoolCreateAPI{}
	s.PatchValue(storage.GetPoolCreateAPI, func(c *storage.PoolCreateCommand) (storage.PoolCreateAPI, error) {
		return s.mockAPI, nil
	})
}

func runPoolCreate(c *gc.C, args []string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&storage.PoolCreateCommand{}), args...)
}

func (s *PoolCreateSuite) TestPoolCreateNameMandatory(c *gc.C) {
	_, err := runPoolCreate(c, []string{"-t", "sunshine"})
	c.Check(err, gc.ErrorMatches, "no pool name specified")
}

func (s *PoolCreateSuite) TestPoolCreateTypeMandatory(c *gc.C) {
	_, err := runPoolCreate(c, []string{""})
	c.Check(err, gc.ErrorMatches, "no provider type for pool specified")
}

func (s *PoolCreateSuite) TestPoolCreateConfigMandatory(c *gc.C) {
	_, err := runPoolCreate(c, []string{"-t", "sunshine", "lollypop"})
	c.Check(err, gc.ErrorMatches, "no pool config specified")
}

func (s *PoolCreateSuite) TestPoolCreate(c *gc.C) {
	_, err := runPoolCreate(c, []string{"-t", "sunshine", "lollypop", "something=too", "another=one"})
	c.Check(err, jc.ErrorIsNil)
}

func (s *PoolCreateSuite) TestPoolCreateSwapPositions(c *gc.C) {
	_, err := runPoolCreate(c, []string{"lollypop", "-t", "sunshine", "something=too", "another=one"})
	c.Check(err, jc.ErrorIsNil)
}

type mockPoolCreateAPI struct {
}

func (s mockPoolCreateAPI) CreatePool(pname, ptype string, pconfig map[string]interface{}) error {
	return nil
}

func (s mockPoolCreateAPI) Close() error {
	return nil
}
