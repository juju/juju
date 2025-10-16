// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v2"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/uniter/hook"
	operationmocks "github.com/juju/juju/internal/worker/uniter/operation/mocks"
	"github.com/juju/juju/internal/worker/uniter/secrets"
	"github.com/juju/juju/internal/worker/uniter/secrets/mocks"
	"github.com/juju/juju/rpc/params"
)

type secretsSuite struct {
	stateReadWriter *operationmocks.MockUnitStateReadWriter
	secretsClient   *mocks.MockSecretsClient
}

func TestSecretsSuite(t *testing.T) {
	tc.Run(t, &secretsSuite{})
}

func (s *secretsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.stateReadWriter = operationmocks.NewMockUnitStateReadWriter(ctrl)
	s.secretsClient = mocks.NewMockSecretsClient(ctrl)
	return ctrl
}

func ptr[T any](v T) *T {
	return &v
}

func (s *secretsSuite) yamlString(c *tc.C, st *secrets.State) string {
	data, err := yaml.Marshal(st)
	c.Assert(err, tc.ErrorIsNil)
	return string(data)
}

func (s *secretsSuite) TestCommitSecretChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.stateReadWriter.EXPECT().State(gomock.Any()).Return(params.UnitStateResult{SecretState: s.yamlString(c,
		&secrets.State{
			ConsumedSecretInfo: map[string]int{
				"secret:666e2mr0ui3e8a215n4g": 664,
				"secret:9m4e2mr0ui3e8a215n4g": 665,
			},
		},
	)}, nil)
	s.secretsClient.EXPECT().GetConsumerSecretsRevisionInfo(
		gomock.Any(), "foo/0",
		[]string{"secret:666e2mr0ui3e8a215n4g", "secret:9m4e2mr0ui3e8a215n4g"}).Return(
		map[string]coresecrets.SecretRevisionInfo{"secret:9m4e2mr0ui3e8a215n4g": {LatestRevision: 667}}, nil,
	)

	s.stateReadWriter.EXPECT().SetState(gomock.Any(), params.SetUnitStateArg{SecretState: ptr(s.yamlString(c,
		&secrets.State{
			ConsumedSecretInfo:      map[string]int{"secret:9m4e2mr0ui3e8a215n4g": 667},
			SecretObsoleteRevisions: map[string][]int{},
		},
	))})

	s.stateReadWriter.EXPECT().SetState(gomock.Any(), params.SetUnitStateArg{SecretState: ptr(s.yamlString(c,
		&secrets.State{
			ConsumedSecretInfo:      map[string]int{"secret:9m4e2mr0ui3e8a215n4g": 666},
			SecretObsoleteRevisions: map[string][]int{},
		},
	))})

	tag := names.NewUnitTag("foo/0")
	tracker, err := secrets.NewSecrets(c.Context(), s.secretsClient, tag, s.stateReadWriter, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	info := hook.Info{
		Kind:           hooks.SecretChanged,
		SecretURI:      "secret:9m4e2mr0ui3e8a215n4g",
		SecretRevision: 666,
	}
	err = tracker.CommitHook(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *secretsSuite) TestCommitSecretRemove(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.stateReadWriter.EXPECT().State(gomock.Any()).Return(params.UnitStateResult{SecretState: s.yamlString(c,
		&secrets.State{
			SecretObsoleteRevisions: map[string][]int{
				"secret:666e2mr0ui3e8a215n4g": {664},
				"secret:9m4e2mr0ui3e8a215n4g": {665},
			},
		},
	)}, nil)

	s.stateReadWriter.EXPECT().SetState(gomock.Any(), params.SetUnitStateArg{SecretState: ptr(s.yamlString(c,
		&secrets.State{
			ConsumedSecretInfo: map[string]int{},
			SecretObsoleteRevisions: map[string][]int{
				"secret:666e2mr0ui3e8a215n4g": {664},
				"secret:9m4e2mr0ui3e8a215n4g": {665, 666}},
		},
	))})

	tag := names.NewUnitTag("foo/0")
	tracker, err := secrets.NewSecrets(c.Context(), s.secretsClient, tag, s.stateReadWriter, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	info := hook.Info{
		Kind:           hooks.SecretRemove,
		SecretURI:      "secret:9m4e2mr0ui3e8a215n4g",
		SecretRevision: 666,
	}
	err = tracker.CommitHook(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *secretsSuite) TestCommitNoOpSecretRevisionRemoved(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.stateReadWriter.EXPECT().State(gomock.Any()).Return(params.UnitStateResult{SecretState: s.yamlString(c,
		&secrets.State{
			SecretObsoleteRevisions: map[string][]int{
				"secret:666e2mr0ui3e8a215n4g": {664},
				"secret:9m4e2mr0ui3e8a215n4g": {665},
				"secret:777e2mr0ui3e8a215n4g": {777},
				"secret:888e2mr0ui3e8a215n4g": {888},
			},
			ConsumedSecretInfo: map[string]int{
				"secret:666e2mr0ui3e8a215n4g": 666,
				"secret:9m4e2mr0ui3e8a215n4g": 667,
				"secret:777e2mr0ui3e8a215n4g": 777,
			},
		},
	)}, nil)
	s.secretsClient.EXPECT().GetConsumerSecretsRevisionInfo(
		gomock.Any(), "foo/0",
		[]string{"secret:666e2mr0ui3e8a215n4g", "secret:777e2mr0ui3e8a215n4g", "secret:9m4e2mr0ui3e8a215n4g"}).Return(
		map[string]coresecrets.SecretRevisionInfo{
			"secret:666e2mr0ui3e8a215n4g": {LatestRevision: 666},
			"secret:9m4e2mr0ui3e8a215n4g": {LatestRevision: 667},
			"secret:777e2mr0ui3e8a215n4g": {LatestRevision: 777},
		}, nil,
	)

	s.stateReadWriter.EXPECT().SetState(gomock.Any(), params.SetUnitStateArg{SecretState: ptr(s.yamlString(c,
		&secrets.State{
			ConsumedSecretInfo: map[string]int{
				"secret:666e2mr0ui3e8a215n4g": 666,
				"secret:9m4e2mr0ui3e8a215n4g": 667,
			},
			SecretObsoleteRevisions: map[string][]int{
				"secret:9m4e2mr0ui3e8a215n4g": {665},
				"secret:888e2mr0ui3e8a215n4g": {888},
			},
		},
	))})

	tag := names.NewUnitTag("foo/0")
	tracker, err := secrets.NewSecrets(c.Context(), s.secretsClient, tag, s.stateReadWriter, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	err = tracker.SecretsRemoved(c.Context(), map[string][]int{
		"secret:666e2mr0ui3e8a215n4g": {664},
		"secret:777e2mr0ui3e8a215n4g": {},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *secretsSuite) TestCollectRemovedSecretObsoleteRevisions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.stateReadWriter.EXPECT().State(gomock.Any()).Return(params.UnitStateResult{SecretState: s.yamlString(c,
		&secrets.State{
			SecretObsoleteRevisions: map[string][]int{
				"secret:666e2mr0ui3e8a215n4g": {664},
				"secret:9m4e2mr0ui3e8a215n4g": {665},
				"secret:777e2mr0ui3e8a215n4g": {777, 778},
				"secret:888e2mr0ui3e8a215n4g": {888, 889},
			},
			ConsumedSecretInfo: map[string]int{
				"secret:666e2mr0ui3e8a215n4g": 666,
				"secret:9m4e2mr0ui3e8a215n4g": 667,
				"secret:777e2mr0ui3e8a215n4g": 777,
			},
		},
	)}, nil)
	s.secretsClient.EXPECT().GetConsumerSecretsRevisionInfo(
		c.Context(), "foo/0",
		[]string{"secret:666e2mr0ui3e8a215n4g", "secret:777e2mr0ui3e8a215n4g", "secret:9m4e2mr0ui3e8a215n4g"}).Return(
		map[string]coresecrets.SecretRevisionInfo{
			"secret:666e2mr0ui3e8a215n4g": {LatestRevision: 666},
			"secret:9m4e2mr0ui3e8a215n4g": {LatestRevision: 667},
			"secret:777e2mr0ui3e8a215n4g": {LatestRevision: 777},
		}, nil,
	)

	tag := names.NewUnitTag("foo/0")
	tracker, err := secrets.NewSecrets(c.Context(), s.secretsClient, tag, s.stateReadWriter, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	res := tracker.CollectRemovedSecretObsoleteRevisions(map[string][]int{
		"secret:9m4e2mr0ui3e8a215n4g": {665, 666},
		"secret:777e2mr0ui3e8a215n4g": {779},
		"secret:888e2mr0ui3e8a215n4g": {889},
	})
	c.Assert(res, tc.DeepEquals, map[string][]int{
		"secret:666e2mr0ui3e8a215n4g": nil,
		"secret:777e2mr0ui3e8a215n4g": {777, 778},
		"secret:888e2mr0ui3e8a215n4g": {888},
	})
}
