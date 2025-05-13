// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/rpc/params"
)

type addSuite struct {
	SubStorageSuite
	mockAPI *mockAddAPI
	args    []string
}

var _ = tc.Suite(&addSuite{})

func (s *addSuite) SetUpTest(c *tc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockAddAPI{
		addToUnitFunc: func(storages []params.StorageAddParams) ([]params.AddStorageResult, error) {
			result := make([]params.AddStorageResult, len(storages))
			for i, one := range storages {
				if strings.HasPrefix(one.StorageName, "err") {
					result[i].Error = apiservererrors.ServerError(errors.Errorf("test failure"))
					continue
				}
				result[i].Result = &params.AddStorageDetails{
					StorageTags: []string{
						"storage-foo-0",
						"storage-foo-1",
					},
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
		expectedErr: `storage directives require at least one field to be specified`,
		visibleErr:  `cannot parse directive for storage "data": storage directives require at least one field to be specified`,
	},
	{
		args:        []string{"tst/123", "data=-676"},
		expectedErr: `count must be greater than zero, got "-676"`,
		visibleErr:  `cannot parse directive for storage "data": cannot parse count: count must be greater than zero, got "-676"`,
	},
	{
		args:        []string{"tst/123", "data=676", "data=676"},
		expectedErr: `storage "data" specified more than once`,
		visibleErr:  `storage "data" specified more than once`,
	},
}

func (s *addSuite) TestAddArgs(c *tc.C) {
	for i, t := range errorTsts {
		c.Logf("test %d for %q", i, t.args)
		s.args = t.args
		s.assertAddErrorOutput(c, t.expectedErr, visibleErrorMessage(t.visibleErr))
	}
}

func (s *addSuite) TestAddInvalidUnit(c *tc.C) {
	s.args = []string{"tst-123", "data=676"}

	expectedErr := `unit name "tst-123" not valid`
	s.assertAddErrorOutput(c, expectedErr, visibleErrorMessage(expectedErr))
}

func (s *addSuite) TestAddSuccess(c *tc.C) {
	validArgs := [][]string{
		{"tst/123", "data=676"},
		{"tst/123", "data"},
	}
	expectedStderr := `
added storage foo/0 to tst/123
added storage foo/1 to tst/123
`[1:]

	for i, args := range validArgs {
		c.Logf("test %d for %q", i, args)
		context, err := s.runAdd(c, args...)
		c.Assert(err, tc.ErrorIsNil)
		s.assertExpectedOutput(c, context, expectedStderr)
	}
}

func (s *addSuite) TestAddOperationAborted(c *tc.C) {
	s.args = []string{"tst/123", "data=676"}
	s.mockAPI.addToUnitFunc = func(storages []params.StorageAddParams) ([]params.AddStorageResult, error) {
		return nil, errors.New("aborted")
	}
	s.assertAddErrorOutput(c, ".*aborted.*", "")
}

func (s *addSuite) TestAddFailure(c *tc.C) {
	s.args = []string{"tst/123", "err=676"}
	s.assertAddErrorOutput(c, "cmd: error out silently", "failed to add storage \"err\" to tst/123: test failure\n")
}

func (s *addSuite) TestAddMixOrderPreserved(c *tc.C) {
	expectedErr := `
added storage foo/0 to tst/123
added storage foo/1 to tst/123
failed to add storage "err" to tst/123: test failure
`[1:]

	s.args = []string{"tst/123", "a=676", "err=676"}
	s.assertAddErrorOutput(c, "cmd: error out silently", expectedErr)

	s.args = []string{"tst/123", "err=676", "a=676"}
	s.assertAddErrorOutput(c, "cmd: error out silently", expectedErr)
}

func (s *addSuite) TestAddAllDistinctErrors(c *tc.C) {
	expectedErr := `
added storage "storage0" to tst/123
added storage "storage1" to tst/123
failed to add storage "storage2" to tst/123: storage pool "barf" not found
failed to add storage "storage42" to tst/123: storage "storage42" not found
`[1:]

	s.args = []string{"tst/123", "storage0=ebs", "storage2=barf", "storage1=123", "storage42=loop"}
	s.mockAPI.addToUnitFunc = func(storages []params.StorageAddParams) ([]params.AddStorageResult, error) {
		result := make([]params.AddStorageResult, len(storages))
		for i, one := range storages {
			if one.StorageName == "storage2" {
				result[i].Error = apiservererrors.ServerError(errors.Errorf(`storage pool "barf" not found`))
			}
			if one.StorageName == "storage42" {
				result[i].Error = apiservererrors.ServerError(errors.Errorf(`storage "storage42" not found`))
			}
		}
		return result, nil
	}

	s.assertAddErrorOutput(c, "cmd: error out silently", expectedErr)
}

func (s *addSuite) TestAddStorageOnlyDistinctErrors(c *tc.C) {
	expectedErr := `
added storage "storage0" to tst/123
failed to add storage "storage2" to tst/123: storage "storage2" not found
failed to add storage "storage42" to tst/123: storage "storage42" not found
`[1:]

	s.args = []string{"tst/123", "storage0=ebs", "storage2=barf", "storage42=loop"}
	s.mockAPI.addToUnitFunc = func(storages []params.StorageAddParams) ([]params.AddStorageResult, error) {
		result := make([]params.AddStorageResult, len(storages))
		for i, one := range storages {
			if one.StorageName == "storage42" || one.StorageName == "storage2" {
				result[i].Error = apiservererrors.ServerError(errors.Errorf(`storage "%v" not found`, one.StorageName))
			}
		}
		return result, nil
	}

	s.assertAddErrorOutput(c, "cmd: error out silently", expectedErr)
}

func (s *addSuite) TestAddStorageMixDistinctAndNonDistinctErrors(c *tc.C) {
	expectedErr := `
some unit error
storage "storage0" not found
`[1:]

	unitErr := `some unit error`
	s.args = []string{"tst/123", "storage0=ebs", "storage2=barf", "storage42=loop"}
	s.mockAPI.addToUnitFunc = func(storages []params.StorageAddParams) ([]params.AddStorageResult, error) {
		result := make([]params.AddStorageResult, len(storages))
		for i, one := range storages {
			if one.StorageName == "storage42" || one.StorageName == "storage2" {
				result[i].Error = apiservererrors.ServerError(errors.New(unitErr))
			} else {
				result[i].Error = apiservererrors.ServerError(errors.Errorf(`storage "%v" not found`, one.StorageName))
			}
		}
		return result, nil
	}

	s.assertAddErrorOutput(c, "cmd: error out silently", expectedErr)
}

func (s *addSuite) TestCollapseUnitErrors(c *tc.C) {
	expectedErr := `some unit error`

	s.args = []string{"tst/123", "storage0=ebs", "storage2=barf", "storage1=123", "storage42=loop"}
	s.mockAPI.addToUnitFunc = func(storages []params.StorageAddParams) ([]params.AddStorageResult, error) {
		result := make([]params.AddStorageResult, len(storages))
		for i := range storages {
			result[i].Error = apiservererrors.ServerError(errors.New(expectedErr))
		}
		return result, nil
	}

	s.assertAddErrorOutput(c, "cmd: error out silently", expectedErr+"\n")
}

func (s *addSuite) TestUnauthorizedMentionsJujuGrant(c *tc.C) {
	s.args = []string{"tst/123", "data"}
	s.mockAPI.addToUnitFunc = func(storages []params.StorageAddParams) ([]params.AddStorageResult, error) {
		return nil, &params.Error{
			Message: "permission denied",
			Code:    params.CodeUnauthorized,
		}
	}

	ctx, _ := s.runAdd(c, s.args...)
	errString := strings.Replace(cmdtesting.Stderr(ctx), "\n", " ", -1)
	c.Assert(errString, tc.Matches, `.*juju grant.*`)
}

func (s *addSuite) assertAddErrorOutput(c *tc.C, expected string, expectedErr string) {
	context, err := s.runAdd(c, s.args...)
	c.Assert(errors.Cause(err), tc.ErrorMatches, expected)
	s.assertExpectedOutput(c, context, expectedErr)
}

func (s *addSuite) assertExpectedOutput(c *tc.C, context *cmd.Context, expectedErr string) {
	c.Assert(cmdtesting.Stdout(context), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, expectedErr)
}

func (s *addSuite) runAdd(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, storage.NewAddCommandForTest(s.mockAPI, s.store), args...)
}

func visibleErrorMessage(errMsg string) string {
	return fmt.Sprintf("ERROR %v\n", errMsg)
}

type mockAddAPI struct {
	addToUnitFunc func(storages []params.StorageAddParams) ([]params.AddStorageResult, error)
}

func (s mockAddAPI) Close() error {
	return nil
}

func (s mockAddAPI) AddToUnit(ctx context.Context, storages []params.StorageAddParams) ([]params.AddStorageResult, error) {
	return s.addToUnitFunc(storages)
}
