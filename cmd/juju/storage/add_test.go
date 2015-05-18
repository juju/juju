// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/storage"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type AddSuite struct {
	SubStorageSuite
	mockAPI *mockAddAPI
	args    []string
}

var _ = gc.Suite(&AddSuite{})

func (s *AddSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockAddAPI{}
	s.PatchValue(storage.GetStorageAddAPI, func(c *storage.AddCommand) (storage.StorageAddAPI, error) {
		return s.mockAPI, nil
	})
	s.args = nil
}

func (s *AddSuite) TestAddNoArgs(c *gc.C) {
	s.assertAddErrorOutput(c, ".*storage add requires a unit and a storage directive.*")
}

func (s *AddSuite) TestAddOnlyUnit(c *gc.C) {
	s.args = []string{"tst/123"}
	s.assertAddErrorOutput(c, ".*storage add requires a unit and a storage directive.*")
}

func (s *AddSuite) TestAddNoConstraints(c *gc.C) {
	s.args = []string{"tst/123", "data"}
	s.assertAddErrorOutput(c, `.*expected "key=value", got "data".*`)
}

func (s *AddSuite) TestAddEmptyConstraints(c *gc.C) {
	s.args = []string{"tst/123", "data="}
	s.assertAddErrorOutput(c, ".*storage constraints require at least one field to be specified.*")
}

func (s *AddSuite) TestAddUnparseableConstraints(c *gc.C) {
	s.args = []string{"tst/123", "data=-676"}
	s.assertAddErrorOutput(c, `.*count must be greater than zero, got "-676".*`)
}

func runAdd(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&storage.AddCommand{}), args...)
}

func (s *AddSuite) TestAddInvalidUnit(c *gc.C) {
	s.args = []string{"tst-123", "data=676"}
	s.assertAddErrorOutput(c, `.*unit name "tst-123" not valid.*`)
}

func (s *AddSuite) assertAddErrorOutput(c *gc.C, expected string) {
	_, err := runAdd(c, s.args...)
	c.Assert(errors.Cause(err), gc.ErrorMatches, expected)
}

func (s *AddSuite) TestAddSuccess(c *gc.C) {
	s.args = []string{"tst/123", "data=676"}
	s.assertAddOutput(c, "", "")
}

func (s *AddSuite) TestAddOperationAborted(c *gc.C) {
	s.args = []string{"tst/123", "data=676"}
	s.mockAPI.abort = true
	s.assertAddErrorOutput(c, ".*aborted.*")
}

func (s *AddSuite) TestAddFailure(c *gc.C) {
	s.args = []string{"tst/123", "err=676"}
	s.assertAddOutput(c, "", "fail: storage \"err\": test failure\n")
}

func (s *AddSuite) TestAddMixOrderPreserved(c *gc.C) {
	expectedErr := `
fail: storage "err": test failure
success: storage "a"`[1:]

	s.args = []string{"tst/123", "a=676", "err=676"}
	s.assertAddOutput(c, "", expectedErr)

	s.args = []string{"tst/123", "err=676", "a=676"}
	s.assertAddOutput(c, "", expectedErr)
}

func (s *AddSuite) assertAddOutput(c *gc.C, expectedValid, expectedErr string) {
	context, err := runAdd(c, s.args...)
	c.Assert(err, jc.ErrorIsNil)

	obtainedErr := testing.Stderr(context)
	c.Assert(obtainedErr, gc.Equals, expectedErr)

	obtainedValid := testing.Stdout(context)
	c.Assert(obtainedValid, gc.Equals, expectedValid)
}

type mockAddAPI struct {
	abort bool
}

func (s mockAddAPI) Close() error {
	return nil
}

func (s mockAddAPI) AddToUnit(storages []params.StorageAddParams) ([]params.ErrorResult, error) {
	if s.abort {
		return nil, errors.New("aborted")
	}
	result := make([]params.ErrorResult, len(storages))
	for i, one := range storages {
		if strings.HasPrefix(one.StorageName, "err") {
			result[i].Error = common.ServerError(fmt.Errorf("test failure"))
		}
	}
	return result, nil
}
