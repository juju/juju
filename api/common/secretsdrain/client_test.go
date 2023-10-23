// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/common/secretsdrain"
	"github.com/juju/juju/api/common/secretsdrain/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&secretsDrainSuite{})

type secretsDrainSuite struct {
	coretesting.BaseSuite
}

func (s *secretsDrainSuite) TestNewClient(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)
	client := secretsdrain.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func (s *secretsDrainSuite) TestGetSecretsToDrain(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	now := time.Now()
	apiCaller.EXPECT().FacadeCall("GetSecretsToDrain", nil, gomock.Any()).SetArg(
		2, params.ListSecretResults{
			Results: []params.ListSecretResult{{
				URI:              uri.String(),
				OwnerTag:         "application-mariadb",
				Label:            "label",
				LatestRevision:   667,
				NextRotateTime:   &now,
				LatestExpireTime: &now,
				Revisions: []params.SecretRevision{{
					Revision: 666,
					ValueRef: &params.SecretValueRef{
						BackendID:  "backend-id",
						RevisionID: "rev-id",
					},
				}, {
					Revision: 667,
				}},
			}},
		},
	).Return(nil)

	client := secretsdrain.NewClient(apiCaller)
	result, err := client.GetSecretsToDrain()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	for _, info := range result {
		c.Assert(info.Metadata.URI.String(), gc.Equals, uri.String())
		c.Assert(info.Metadata.OwnerTag, gc.Equals, "application-mariadb")
		c.Assert(info.Metadata.Label, gc.Equals, "label")
		c.Assert(info.Metadata.LatestRevision, gc.Equals, 667)
		c.Assert(info.Metadata.LatestExpireTime, gc.Equals, &now)
		c.Assert(info.Metadata.NextRotateTime, gc.Equals, &now)
		c.Assert(info.Revisions, jc.DeepEquals, []coresecrets.SecretRevisionMetadata{
			{
				Revision: 666,
				ValueRef: &coresecrets.ValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				},
			},
			{
				Revision: 667,
			},
		})
	}
}

func (s *secretsDrainSuite) TestChangeSecretBackend(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(
		"ChangeSecretBackend",
		params.ChangeSecretBackendArgs{
			Args: []params.ChangeSecretBackendArg{
				{
					URI:      uri.String(),
					Revision: 666,
					Content: params.SecretContentParams{
						ValueRef: &params.SecretValueRef{
							BackendID:  "backend-id",
							RevisionID: "rev-id",
						},
					},
				},
			},
		},
		gomock.Any(),
	).SetArg(
		2, params.ErrorResults{
			[]params.ErrorResult{
				{Error: nil},
			},
		},
	).Return(nil)

	client := secretsdrain.NewClient(apiCaller)
	result, err := client.ChangeSecretBackend(
		[]secretsdrain.ChangeSecretBackendArg{
			{
				URI:      uri,
				Revision: 666,
				ValueRef: &coresecrets.ValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				},
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], jc.ErrorIsNil)
}

func (s *secretsDrainSuite) TestWatchSecretBackendChanged(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	apiCaller.EXPECT().FacadeCall("WatchSecretBackendChanged", nil, gomock.Any()).SetArg(
		2, params.NotifyWatchResult{
			Error: &params.Error{Message: "FAIL"},
		},
	).Return(nil)

	client := secretsdrain.NewClient(apiCaller)
	_, err := client.WatchSecretBackendChanged()
	c.Assert(err, gc.ErrorMatches, "FAIL")
}
