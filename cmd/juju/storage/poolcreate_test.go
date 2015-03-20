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

func (s *PoolCreateSuite) TestPoolCreateOneArg(c *gc.C) {
	_, err := runPoolCreate(c, []string{"sunshine"})
	c.Check(err, gc.ErrorMatches, "pool creation requires names, provider type and attrs for configuration")
}

func (s *PoolCreateSuite) TestPoolCreateNoArgs(c *gc.C) {
	_, err := runPoolCreate(c, []string{""})
	c.Check(err, gc.ErrorMatches, "pool creation requires names, provider type and attrs for configuration")
}

func (s *PoolCreateSuite) TestPoolCreateTwoArgs(c *gc.C) {
	_, err := runPoolCreate(c, []string{"sunshine", "lollypop"})
	c.Check(err, gc.ErrorMatches, "pool creation requires names, provider type and attrs for configuration")
}

func (s *PoolCreateSuite) TestPoolCreateAttrMissingKey(c *gc.C) {
	_, err := runPoolCreate(c, []string{"sunshine", "lollypop", "=too"})
	c.Check(err, gc.ErrorMatches, `expected "key=value", got "=too"`)
}

func (s *PoolCreateSuite) TestPoolCreateAttrMissingValue(c *gc.C) {
	_, err := runPoolCreate(c, []string{"sunshine", "lollypop", "something="})
	c.Check(err, gc.ErrorMatches, `expected "key=value", got "something="`)
}

func (s *PoolCreateSuite) TestPoolCreateAttrEmptyValue(c *gc.C) {
	_, err := runPoolCreate(c, []string{"sunshine", "lollypop", `something=""`})
	c.Check(err, jc.ErrorIsNil)
}

func (s *PoolCreateSuite) TestPoolCreateOneAttr(c *gc.C) {
	_, err := runPoolCreate(c, []string{"sunshine", "lollypop", "something=too"})
	c.Check(err, jc.ErrorIsNil)
}

func (s *PoolCreateSuite) TestPoolCreateEmptyAttr(c *gc.C) {
	_, err := runPoolCreate(c, []string{"sunshine", "lollypop", ""})
	c.Check(err, gc.ErrorMatches, `expected "key=value", got ""`)
}

func (s *PoolCreateSuite) TestPoolCreateManyAttrs(c *gc.C) {
	_, err := runPoolCreate(c, []string{"sunshine", "lollypop", "something=too", "another=one"})
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
