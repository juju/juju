// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiapplication "github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type RemoveUnitSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake *fakeApplicationRemoveUnitAPI

	store *jujuclient.MemStore
}

var _ = gc.Suite(&RemoveUnitSuite{})

type fakeApplicationRemoveUnitAPI struct {
	application.RemoveApplicationAPI

	units          []string
	scale          int
	destroyStorage bool
	bestAPIVersion int
	err            error
}

func (f *fakeApplicationRemoveUnitAPI) BestAPIVersion() int {
	return f.bestAPIVersion
}

func (f *fakeApplicationRemoveUnitAPI) Close() error {
	return nil
}

func (f *fakeApplicationRemoveUnitAPI) ModelUUID() string {
	return "fake-uuid"
}

func (f *fakeApplicationRemoveUnitAPI) DestroyUnits(args apiapplication.DestroyUnitsParams) ([]params.DestroyUnitResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.units = args.Units
	f.destroyStorage = args.DestroyStorage
	var result []params.DestroyUnitResult
	for _, u := range args.Units {
		var info *params.DestroyUnitInfo
		var err *params.Error
		switch u {
		case "unit/0":
			st := []params.Entity{{Tag: "storage-data-0"}}
			info = &params.DestroyUnitInfo{}
			if f.destroyStorage {
				info.DestroyedStorage = st
			} else {
				info.DetachedStorage = st
			}
		case "unit/1":
			st := []params.Entity{{Tag: "storage-data-1"}}
			info = &params.DestroyUnitInfo{}
			if f.destroyStorage {
				info.DestroyedStorage = st
			} else {
				info.DetachedStorage = st
			}
		case "unit/2":
			err = &params.Error{Code: params.CodeNotFound, Message: `unit "unit/2" does not exist`}
		}
		result = append(result, params.DestroyUnitResult{
			Info:  info,
			Error: err,
		})
	}
	return result, nil
}

func (f *fakeApplicationRemoveUnitAPI) ScaleApplication(args apiapplication.ScaleApplicationParams) (params.ScaleApplicationResult, error) {
	if f.err != nil {
		return params.ScaleApplicationResult{}, f.err
	}
	f.scale += args.ScaleChange
	return params.ScaleApplicationResult{
		Info: &params.ScaleApplicationInfo{
			Scale: f.scale,
		},
	}, nil
}

func (s *RemoveUnitSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fake = &fakeApplicationRemoveUnitAPI{
		bestAPIVersion: 5,
		scale:          5,
	}
	s.store = jujuclienttesting.MinimalStore()
}

func (s *RemoveUnitSuite) runRemoveUnit(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, application.NewRemoveUnitCommandForTest(s.fake, s.store), args...)
}

func (s *RemoveUnitSuite) TestRemoveUnit(c *gc.C) {
	ctx, err := s.runRemoveUnit(c, "unit/0", "unit/1", "unit/2")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(s.fake.units, jc.DeepEquals, []string{"unit/0", "unit/1", "unit/2"})
	c.Assert(s.fake.destroyStorage, jc.IsFalse)

	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
removing unit unit/0
- will detach storage data/0
removing unit unit/1
- will detach storage data/1
removing unit unit/2 failed: unit "unit/2" does not exist
`[1:])
}

func (s *RemoveUnitSuite) TestRemoveUnitDestroyStorage(c *gc.C) {
	ctx, err := s.runRemoveUnit(c, "unit/0", "unit/1", "unit/2", "--destroy-storage")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Assert(s.fake.units, jc.DeepEquals, []string{"unit/0", "unit/1", "unit/2"})
	c.Assert(s.fake.destroyStorage, jc.IsTrue)

	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
removing unit unit/0
- will remove storage data/0
removing unit unit/1
- will remove storage data/1
removing unit unit/2 failed: unit "unit/2" does not exist
`[1:])
}

func (s *RemoveUnitSuite) TestRemoveUnitNoWaitWithoutForce(c *gc.C) {
	_, err := s.runRemoveUnit(c, "unit/0", "--no-wait")
	c.Assert(err, gc.ErrorMatches, `--no-wait without --force not valid`)
}

func (s *RemoveUnitSuite) TestBlockRemoveUnit(c *gc.C) {
	// Block operation
	s.fake.err = common.OperationBlockedError("TestBlockRemoveUnit")
	s.runRemoveUnit(c, "some-unit-name/0")

	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*TestBlockRemoveUnit.*")
}

func (s *RemoveUnitSuite) TestCAASRemoveUnit(c *gc.C) {
	m := s.store.Models["arthur"].Models["king/sword"]
	m.ModelType = model.CAAS
	s.store.Models["arthur"].Models["king/sword"] = m

	ctx, err := s.runRemoveUnit(c, "some-application-name", "--num-units", "2")
	c.Assert(err, jc.ErrorIsNil)

	stderr := cmdtesting.Stderr(ctx)
	c.Assert(stderr, gc.Equals, `
scaling down to 3 units
`[1:])
}

func (s *RemoveUnitSuite) TestCAASRemoveUnitNotSupported(c *gc.C) {
	m := s.store.Models["arthur"].Models["king/sword"]
	m.ModelType = model.CAAS
	s.store.Models["arthur"].Models["king/sword"] = m

	s.fake.err = common.ServerError(errors.NotSupportedf(`scale a "daemon" charm`))

	_, err := s.runRemoveUnit(c, "some-application-name", "--num-units", "2")
	c.Assert(err, gc.ErrorMatches, `can not remove unit: scale a "daemon" charm not supported`)
}

func (s *RemoveUnitSuite) TestCAASAllowsNumUnitsOnly(c *gc.C) {
	m := s.store.Models["arthur"].Models["king/sword"]
	m.ModelType = model.CAAS
	s.store.Models["arthur"].Models["king/sword"] = m

	_, err := s.runRemoveUnit(c, "some-application-name")
	c.Assert(err, gc.ErrorMatches, "removing 0 units not valid")

	_, err = s.runRemoveUnit(c, "some-application-name", "--destroy-storage")
	c.Assert(err, gc.ErrorMatches, "k8s models only support --num-units")

	_, err = s.runRemoveUnit(c, "some-application-name/0")
	c.Assert(err, gc.NotNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, gc.Matches, "k8s models do not support removing named units.*")

	_, err = s.runRemoveUnit(c, "some-application-name-", "--num-units", "2")
	c.Assert(err, gc.ErrorMatches, "application name \"some-application-name-\" not valid")

	_, err = s.runRemoveUnit(c, "some-application-name", "another-application", "--num-units", "2")
	c.Assert(err, gc.ErrorMatches, "only single application supported")

	_, err = s.runRemoveUnit(c, "some-application-name", "--num-units", "2")
	c.Assert(err, jc.ErrorIsNil)
}
