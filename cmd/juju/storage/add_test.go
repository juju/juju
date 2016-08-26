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
	"github.com/juju/juju/cmd/juju/storage"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type addSuite struct {
	SubStorageSuite
	mockAPI *mockAddAPI
	args    []string
}

var _ = gc.Suite(&addSuite{})

func (s *addSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockAddAPI{
		addToUnitFunc: func(storages []params.StorageAddParams) ([]params.ErrorResult, error) {
			result := make([]params.ErrorResult, len(storages))
			for i, one := range storages {
				if strings.HasPrefix(one.StorageName, "err") {
					result[i].Error = common.ServerError(errors.Errorf("test failure"))
				}
			}
			return result, nil
		},
	}
	s.args = nil
}

type tstData struct {
	args        []string
	expectedErr string
	visibleErr  string
}

var errorTsts = []tstData{
	{
		args:        nil,
		expectedErr: "add-storage requires a unit and a storage directive",
		visibleErr:  "add-storage requires a unit and a storage directive",
	},
	{
		args:        []string{"tst/123"},
		expectedErr: "add-storage requires a unit and a storage directive",
		visibleErr:  "add-storage requires a unit and a storage directive",
	},
	{
		args:        []string{"tst/123", "data="},
		expectedErr: `storage constraints require at least one field to be specified`,
		visibleErr:  `cannot parse constraints for storage "data": storage constraints require at least one field to be specified`,
	},
	{
		args:        []string{"tst/123", "data=-676"},
		expectedErr: `count must be greater than zero, got "-676"`,
		visibleErr:  `cannot parse constraints for storage "data": cannot parse count: count must be greater than zero, got "-676"`,
	},
	{
		args:        []string{"tst/123", "data=676", "data=676"},
		expectedErr: `storage "data" specified more than once`,
		visibleErr:  `storage "data" specified more than once`,
	},
}

func (s *addSuite) TestAddArgs(c *gc.C) {
	for i, t := range errorTsts {
		c.Logf("test %d for %q", i, t.args)
		s.args = t.args
		s.assertAddErrorOutput(c, t.expectedErr, "", visibleErrorMessage(t.visibleErr))
	}
}

func (s *addSuite) TestAddInvalidUnit(c *gc.C) {
	s.args = []string{"tst-123", "data=676"}

	expectedErr := `unit name "tst-123" not valid`
	s.assertAddErrorOutput(c, expectedErr, "", visibleErrorMessage(expectedErr))
}

var validArgs = [][]string{
	[]string{"tst/123", "data=676"},
	[]string{"tst/123", "data"},
}

func (s *addSuite) TestAddSuccess(c *gc.C) {
	for i, args := range validArgs {
		c.Logf("test %d for %q", i, args)
		s.args = args
		s.assertAddOutput(c, "added \"data\"\n", "")
	}
}

func (s *addSuite) TestAddOperationAborted(c *gc.C) {
	s.args = []string{"tst/123", "data=676"}
	s.mockAPI.addToUnitFunc = func(storages []params.StorageAddParams) ([]params.ErrorResult, error) {
		return nil, errors.New("aborted")
	}
	s.assertAddErrorOutput(c, ".*aborted.*", "", "")
}

func (s *addSuite) TestAddFailure(c *gc.C) {
	s.args = []string{"tst/123", "err=676"}
	s.assertAddErrorOutput(c, "cmd: error out silently", "", "failed to add \"err\": test failure\n")
}

func (s *addSuite) TestAddMixOrderPreserved(c *gc.C) {
	expectedOut := `
added "a"
`[1:]
	expectedErr := `
failed to add "err": test failure
`[1:]

	s.args = []string{"tst/123", "a=676", "err=676"}
	s.assertAddErrorOutput(c, "cmd: error out silently", expectedOut, expectedErr)

	s.args = []string{"tst/123", "err=676", "a=676"}
	s.assertAddErrorOutput(c, "cmd: error out silently", expectedOut, expectedErr)
}

func (s *addSuite) TestAddAllDistinctErrors(c *gc.C) {
	expectedOut := `
added "storage0"
added "storage1"
`[1:]
	expectedErr := `
failed to add "storage2": storage pool "barf" not found
failed to add "storage42": storage "storage42" not found
`[1:]

	s.args = []string{"tst/123", "storage0=ebs", "storage2=barf", "storage1=123", "storage42=loop"}
	s.mockAPI.addToUnitFunc = func(storages []params.StorageAddParams) ([]params.ErrorResult, error) {
		result := make([]params.ErrorResult, len(storages))
		for i, one := range storages {
			if one.StorageName == "storage2" {
				result[i].Error = common.ServerError(errors.Errorf(`storage pool "barf" not found`))
			}
			if one.StorageName == "storage42" {
				result[i].Error = common.ServerError(errors.Errorf(`storage "storage42" not found`))
			}
		}
		return result, nil
	}

	s.assertAddErrorOutput(c, "cmd: error out silently", expectedOut, expectedErr)
}

func (s *addSuite) TestAddStorageOnlyDistinctErrors(c *gc.C) {
	expectedOut := `
added "storage0"
`[1:]
	expectedErr := `
failed to add "storage2": storage "storage2" not found
failed to add "storage42": storage "storage42" not found
`[1:]

	s.args = []string{"tst/123", "storage0=ebs", "storage2=barf", "storage42=loop"}
	s.mockAPI.addToUnitFunc = func(storages []params.StorageAddParams) ([]params.ErrorResult, error) {
		result := make([]params.ErrorResult, len(storages))
		for i, one := range storages {
			if one.StorageName == "storage42" || one.StorageName == "storage2" {
				result[i].Error = common.ServerError(errors.Errorf(`storage "%v" not found`, one.StorageName))
			}
		}
		return result, nil
	}

	s.assertAddErrorOutput(c, "cmd: error out silently", expectedOut, expectedErr)
}

func (s *addSuite) TestAddStorageMixDistinctAndNonDistinctErrors(c *gc.C) {
	expectedOut := ``
	expectedErr := `
some unit error
storage "storage0" not found
`[1:]

	unitErr := `some unit error`
	s.args = []string{"tst/123", "storage0=ebs", "storage2=barf", "storage42=loop"}
	s.mockAPI.addToUnitFunc = func(storages []params.StorageAddParams) ([]params.ErrorResult, error) {
		result := make([]params.ErrorResult, len(storages))
		for i, one := range storages {
			if one.StorageName == "storage42" || one.StorageName == "storage2" {
				result[i].Error = common.ServerError(errors.New(unitErr))
			} else {
				result[i].Error = common.ServerError(errors.Errorf(`storage "%v" not found`, one.StorageName))
			}
		}
		return result, nil
	}

	s.assertAddErrorOutput(c, "cmd: error out silently", expectedOut, expectedErr)
}

func (s *addSuite) TestCollapseUnitErrors(c *gc.C) {
	expectedErr := `some unit error`

	s.args = []string{"tst/123", "storage0=ebs", "storage2=barf", "storage1=123", "storage42=loop"}
	s.mockAPI.addToUnitFunc = func(storages []params.StorageAddParams) ([]params.ErrorResult, error) {
		result := make([]params.ErrorResult, len(storages))
		for i, _ := range storages {
			result[i].Error = common.ServerError(errors.New(expectedErr))
		}
		return result, nil
	}

	s.assertAddErrorOutput(c, "cmd: error out silently", "", fmt.Sprintf("%v\n", expectedErr))
}

func (s *addSuite) assertAddOutput(c *gc.C, expectedOut, expectedErr string) {
	context, err := s.runAdd(c, s.args...)
	c.Assert(err, jc.ErrorIsNil)

	s.assertExpectedOutput(c, context, expectedOut, expectedErr)
}

func (s *addSuite) assertAddErrorOutput(c *gc.C, expected string, expectedOut, expectedErr string) {
	context, err := s.runAdd(c, s.args...)
	c.Assert(errors.Cause(err), gc.ErrorMatches, expected)
	s.assertExpectedOutput(c, context, expectedOut, expectedErr)
}

func (s *addSuite) assertExpectedOutput(c *gc.C, context *cmd.Context, expectedOut, expectedErr string) {
	obtainedErr := testing.Stderr(context)
	c.Assert(obtainedErr, gc.Equals, expectedErr)

	obtainedValid := testing.Stdout(context)
	c.Assert(obtainedValid, gc.Equals, expectedOut)
}

func (s *addSuite) runAdd(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, storage.NewAddCommandForTest(s.mockAPI, s.store), args...)
}

func visibleErrorMessage(errMsg string) string {
	return fmt.Sprintf("error: %v\n", errMsg)
}

type mockAddAPI struct {
	addToUnitFunc func(storages []params.StorageAddParams) ([]params.ErrorResult, error)
}

func (s mockAddAPI) Close() error {
	return nil
}

func (s mockAddAPI) AddToUnit(storages []params.StorageAddParams) ([]params.ErrorResult, error) {
	return s.addToUnitFunc(storages)
}
