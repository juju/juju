// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/common/secretsdrain"
	"github.com/juju/juju/api/common/secretsdrain/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestSecretsDrainSuite(t *stdtesting.T) { tc.Run(t, &secretsDrainSuite{}) }

type secretsDrainSuite struct {
	coretesting.BaseSuite
}

func (s *secretsDrainSuite) TestNewClient(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)
	client := secretsdrain.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)
}

func (s *secretsDrainSuite) TestGetSecretsToDrain(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(gomock.Any(), "GetSecretsToDrain", nil, gomock.Any()).SetArg(
		3, params.SecretRevisionsToDrainResults{
			Results: []params.SecretRevisionsToDrainResult{{
				URI: uri.String(),
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
	result, err := client.GetSecretsToDrain(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	for _, info := range result {
		c.Assert(info.URI.String(), tc.Equals, uri.String())
		c.Assert(info.Revisions, tc.DeepEquals, []coresecrets.SecretExternalRevision{
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

func (s *secretsDrainSuite) TestChangeSecretBackend(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	uri := coresecrets.NewURI()
	apiCaller.EXPECT().FacadeCall(
		gomock.Any(),
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
		3, params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: nil},
			},
		},
	).Return(nil)

	client := secretsdrain.NewClient(apiCaller)
	result, err := client.ChangeSecretBackend(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0], tc.ErrorIsNil)
}

func (s *secretsDrainSuite) TestWatchSecretBackendChanged(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	apiCaller := mocks.NewMockFacadeCaller(ctrl)

	apiCaller.EXPECT().FacadeCall(gomock.Any(), "WatchSecretBackendChanged", nil, gomock.Any()).SetArg(
		3, params.NotifyWatchResult{
			Error: &params.Error{Message: "FAIL"},
		},
	).Return(nil)

	client := secretsdrain.NewClient(apiCaller)
	_, err := client.WatchSecretBackendChanged(c.Context())
	c.Assert(err, tc.ErrorMatches, "FAIL")
}
