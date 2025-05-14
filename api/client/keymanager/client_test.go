// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keymanager_test

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/ssh"
	sshtesting "github.com/juju/utils/v4/ssh/testing"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/keymanager"
	"github.com/juju/juju/rpc/params"
)

type keymanagerSuite struct {
}

var _ = tc.Suite(&keymanagerSuite{})

func (s *keymanagerSuite) TestListKeys(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tag, _ := names.ParseUserTag("owner")
	args := params.ListSSHKeys{
		Entities: params.Entities{
			Entities: []params.Entity{{tag.Name()}},
		},
		Mode: params.SSHListModeFingerprint,
	}
	result := new(params.StringsResults)
	results := params.StringsResults{
		Results: []params.StringsResult{
			{Result: []string{sshtesting.ValidKeyOne.Fingerprint + " (user@host)", sshtesting.ValidKeyTwo.Fingerprint}},
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListKeys", args, result).SetArg(3, results).Return(nil)

	client := keymanager.NewClientFromCaller(mockFacadeCaller)
	keyResults, err := client.ListKeys(c.Context(), ssh.Fingerprints, tag.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(keyResults), tc.Equals, 1)
	res := keyResults[0]
	c.Assert(res.Error, tc.IsNil)
	c.Assert(res.Result, tc.DeepEquals,
		[]string{sshtesting.ValidKeyOne.Fingerprint + " (user@host)", sshtesting.ValidKeyTwo.Fingerprint})
}

func (s *keymanagerSuite) TestAddKeys(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	newKeys := []string{sshtesting.ValidKeyTwo.Key, sshtesting.ValidKeyThree.Key, "invalid"}
	tag, _ := names.ParseUserTag("owner")
	args := params.ModifyUserSSHKeys{
		Keys: newKeys,
		User: tag.Name(),
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{},
			{Error: clientError("invalid ssh key: invalid")},
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddKeys", args, result).SetArg(3, results).Return(nil)

	client := keymanager.NewClientFromCaller(mockFacadeCaller)
	errResults, err := client.AddKeys(c.Context(), tag.Name(), newKeys...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errResults, tc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
		{Error: clientError("invalid ssh key: invalid")},
	})
}

func (s *keymanagerSuite) TestDeleteKeys(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tag, _ := names.ParseUserTag("owner")
	args := params.ModifyUserSSHKeys{
		Keys: []string{sshtesting.ValidKeyTwo.Fingerprint, "user@host", "missing"},
		User: tag.Name(),
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{},
			{Error: clientError("invalid ssh key: missing")},
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DeleteKeys", args, result).SetArg(3, results).Return(nil)

	client := keymanager.NewClientFromCaller(mockFacadeCaller)
	errResults, err := client.DeleteKeys(c.Context(), tag.Name(), sshtesting.ValidKeyTwo.Fingerprint, "user@host", "missing")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errResults, tc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
		{Error: clientError("invalid ssh key: missing")},
	})
}

func (s *keymanagerSuite) TestImportKeys(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tag, _ := names.ParseUserTag("owner")
	keyIds := []string{"lp:validuser", "invalid-key"}
	args := params.ModifyUserSSHKeys{
		Keys: keyIds,
		User: tag.Name(),
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{Error: clientError("invalid ssh key id: invalid-key")},
		},
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ImportKeys", args, result).SetArg(3, results).Return(nil)

	client := keymanager.NewClientFromCaller(mockFacadeCaller)
	errResults, err := client.ImportKeys(c.Context(), tag.Name(), keyIds...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errResults, tc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: clientError("invalid ssh key id: invalid-key")},
	})
}

func clientError(message string) *params.Error {
	return &params.Error{
		Message: message,
		Code:    "",
	}
}
