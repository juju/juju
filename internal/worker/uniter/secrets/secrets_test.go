// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/juju/charm/v12/hooks"
	"github.com/juju/loggo"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	coresecrets "github.com/juju/juju/core/secrets"
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

var _ = gc.Suite(&secretsSuite{})

func (s *secretsSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.stateReadWriter = operationmocks.NewMockUnitStateReadWriter(ctrl)
	s.secretsClient = mocks.NewMockSecretsClient(ctrl)
	return ctrl
}

func ptr[T any](v T) *T {
	return &v
}

func (s *secretsSuite) yamlString(c *gc.C, st *secrets.State) string {
	data, err := yaml.Marshal(st)
	c.Assert(err, jc.ErrorIsNil)
	return string(data)
}

func (s *secretsSuite) TestCommitSecretChanged(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.stateReadWriter.EXPECT().State().Return(params.UnitStateResult{SecretState: s.yamlString(c,
		&secrets.State{
			ConsumedSecretInfo: map[string]int{
				"secret:666e2mr0ui3e8a215n4g": 664,
				"secret:9m4e2mr0ui3e8a215n4g": 665,
			},
		},
	)}, nil)
	s.secretsClient.EXPECT().GetConsumerSecretsRevisionInfo("foo/0",
		[]string{"secret:666e2mr0ui3e8a215n4g", "secret:9m4e2mr0ui3e8a215n4g"}).Return(
		map[string]coresecrets.SecretRevisionInfo{"secret:9m4e2mr0ui3e8a215n4g": {Revision: 667}}, nil,
	)

	s.stateReadWriter.EXPECT().SetState(params.SetUnitStateArg{SecretState: ptr(s.yamlString(c,
		&secrets.State{
			ConsumedSecretInfo:      map[string]int{"secret:9m4e2mr0ui3e8a215n4g": 667},
			SecretObsoleteRevisions: map[string][]int{},
		},
	))})

	s.stateReadWriter.EXPECT().SetState(params.SetUnitStateArg{SecretState: ptr(s.yamlString(c,
		&secrets.State{
			ConsumedSecretInfo:      map[string]int{"secret:9m4e2mr0ui3e8a215n4g": 666},
			SecretObsoleteRevisions: map[string][]int{},
		},
	))})

	tag := names.NewUnitTag("foo/0")
	tracker, err := secrets.NewSecrets(s.secretsClient, tag, s.stateReadWriter, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)

	info := hook.Info{
		Kind:           hooks.SecretChanged,
		SecretURI:      "secret:9m4e2mr0ui3e8a215n4g",
		SecretRevision: 666,
	}
	err = tracker.CommitHook(info)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *secretsSuite) TestCommitSecretRemove(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.stateReadWriter.EXPECT().State().Return(params.UnitStateResult{SecretState: s.yamlString(c,
		&secrets.State{
			SecretObsoleteRevisions: map[string][]int{
				"secret:666e2mr0ui3e8a215n4g": {664},
				"secret:9m4e2mr0ui3e8a215n4g": {665},
			},
		},
	)}, nil)

	s.stateReadWriter.EXPECT().SetState(gomock.Any()).DoAndReturn(func(arg0 params.SetUnitStateArg) error {
		var st secrets.State
		err := yaml.Unmarshal([]byte(*arg0.SecretState), &st)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(st, jc.DeepEquals, secrets.State{
			ConsumedSecretInfo: map[string]int{},
			SecretObsoleteRevisions: map[string][]int{
				"secret:666e2mr0ui3e8a215n4g": {664},
				"secret:9m4e2mr0ui3e8a215n4g": {665, 666},
			},
		})
		return nil
	})

	tag := names.NewUnitTag("foo/0")
	tracker, err := secrets.NewSecrets(s.secretsClient, tag, s.stateReadWriter, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)

	info := hook.Info{
		Kind:           hooks.SecretRemove,
		SecretURI:      "secret:9m4e2mr0ui3e8a215n4g",
		SecretRevision: 666,
	}
	err = tracker.CommitHook(info)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *secretsSuite) TestCommitNoOpSecretRevisionRemoved(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.stateReadWriter.EXPECT().State().Return(params.UnitStateResult{SecretState: s.yamlString(c,
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
	s.secretsClient.EXPECT().GetConsumerSecretsRevisionInfo("foo/0",
		[]string{"secret:666e2mr0ui3e8a215n4g", "secret:777e2mr0ui3e8a215n4g", "secret:9m4e2mr0ui3e8a215n4g"}).Return(
		map[string]coresecrets.SecretRevisionInfo{
			"secret:666e2mr0ui3e8a215n4g": {Revision: 666},
			"secret:9m4e2mr0ui3e8a215n4g": {Revision: 667},
			"secret:777e2mr0ui3e8a215n4g": {Revision: 777},
		}, nil,
	)

	s.stateReadWriter.EXPECT().SetState(gomock.Any()).DoAndReturn(func(arg0 params.SetUnitStateArg) error {
		var st secrets.State
		err := yaml.Unmarshal([]byte(*arg0.SecretState), &st)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(st, jc.DeepEquals, secrets.State{
			ConsumedSecretInfo: map[string]int{
				"secret:666e2mr0ui3e8a215n4g": 666,
				"secret:9m4e2mr0ui3e8a215n4g": 667,
			},
			SecretObsoleteRevisions: map[string][]int{
				"secret:9m4e2mr0ui3e8a215n4g": {665},
				"secret:888e2mr0ui3e8a215n4g": {888},
			},
		})
		return nil
	})

	tag := names.NewUnitTag("foo/0")
	tracker, err := secrets.NewSecrets(s.secretsClient, tag, s.stateReadWriter, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)

	err = tracker.SecretsRemoved(map[string][]int{
		"secret:666e2mr0ui3e8a215n4g": {664},
		"secret:777e2mr0ui3e8a215n4g": {},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *secretsSuite) TestCollectRemovedSecretObsoleteRevisions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.stateReadWriter.EXPECT().State().Return(params.UnitStateResult{SecretState: s.yamlString(c,
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
	s.secretsClient.EXPECT().GetConsumerSecretsRevisionInfo("foo/0",
		[]string{"secret:666e2mr0ui3e8a215n4g", "secret:777e2mr0ui3e8a215n4g", "secret:9m4e2mr0ui3e8a215n4g"}).Return(
		map[string]coresecrets.SecretRevisionInfo{
			"secret:666e2mr0ui3e8a215n4g": {Revision: 666},
			"secret:9m4e2mr0ui3e8a215n4g": {Revision: 667},
			"secret:777e2mr0ui3e8a215n4g": {Revision: 777},
		}, nil,
	)

	tag := names.NewUnitTag("foo/0")
	tracker, err := secrets.NewSecrets(s.secretsClient, tag, s.stateReadWriter, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)

	res := tracker.CollectRemovedSecretObsoleteRevisions(map[string][]int{
		"secret:9m4e2mr0ui3e8a215n4g": {665, 666},
		"secret:777e2mr0ui3e8a215n4g": {779},
		"secret:888e2mr0ui3e8a215n4g": {889},
	})
	c.Assert(res, jc.DeepEquals, map[string][]int{
		"secret:666e2mr0ui3e8a215n4g": nil,
		"secret:777e2mr0ui3e8a215n4g": {777, 778},
		"secret:888e2mr0ui3e8a215n4g": {888},
	})
}
