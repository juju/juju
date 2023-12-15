// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/juju/charm/v11/hooks"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/worker/uniter/hook"
	operationmocks "github.com/juju/juju/worker/uniter/operation/mocks"
	"github.com/juju/juju/worker/uniter/secrets"
	"github.com/juju/juju/worker/uniter/secrets/mocks"
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
	s.secretsClient.EXPECT().SecretMetadata().Return(nil, nil)

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
	s.secretsClient.EXPECT().SecretMetadata().Return(
		[]coresecrets.SecretOwnerMetadata{{Metadata: coresecrets.SecretMetadata{URI: &coresecrets.URI{ID: "9m4e2mr0ui3e8a215n4g"}}}}, nil)
	s.stateReadWriter.EXPECT().SetState(params.SetUnitStateArg{SecretState: ptr(s.yamlString(c,
		&secrets.State{
			ConsumedSecretInfo: map[string]int{},
			SecretObsoleteRevisions: map[string][]int{
				"secret:9m4e2mr0ui3e8a215n4g": {665}},
		},
	))})

	s.stateReadWriter.EXPECT().SetState(params.SetUnitStateArg{SecretState: ptr(s.yamlString(c,
		&secrets.State{
			ConsumedSecretInfo: map[string]int{},
			SecretObsoleteRevisions: map[string][]int{
				"secret:9m4e2mr0ui3e8a215n4g": {665, 666}},
		},
	))})

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

func (s *secretsSuite) TestCommitNoOpSecretsRemoved(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.stateReadWriter.EXPECT().State().Return(params.UnitStateResult{SecretState: s.yamlString(c,
		&secrets.State{
			SecretObsoleteRevisions: map[string][]int{
				"secret:666e2mr0ui3e8a215n4g": {664},
				"secret:9m4e2mr0ui3e8a215n4g": {665},
			},
			ConsumedSecretInfo: map[string]int{
				"secret:666e2mr0ui3e8a215n4g": 666,
				"secret:9m4e2mr0ui3e8a215n4g": 667,
			},
		},
	)}, nil)
	s.secretsClient.EXPECT().GetConsumerSecretsRevisionInfo("foo/0",
		[]string{"secret:666e2mr0ui3e8a215n4g", "secret:9m4e2mr0ui3e8a215n4g"}).Return(
		map[string]coresecrets.SecretRevisionInfo{
			"secret:666e2mr0ui3e8a215n4g": {Revision: 666},
			"secret:9m4e2mr0ui3e8a215n4g": {Revision: 667},
		}, nil,
	)
	s.secretsClient.EXPECT().SecretMetadata().Return(
		[]coresecrets.SecretOwnerMetadata{
			{Metadata: coresecrets.SecretMetadata{URI: &coresecrets.URI{ID: "9m4e2mr0ui3e8a215n4g"}}},
			{Metadata: coresecrets.SecretMetadata{URI: &coresecrets.URI{ID: "666e2mr0ui3e8a215n4g"}}},
		}, nil)
	s.stateReadWriter.EXPECT().SetState(params.SetUnitStateArg{SecretState: ptr(s.yamlString(c,
		&secrets.State{
			ConsumedSecretInfo: map[string]int{
				"secret:9m4e2mr0ui3e8a215n4g": 667,
			},
			SecretObsoleteRevisions: map[string][]int{
				"secret:9m4e2mr0ui3e8a215n4g": {665}},
		},
	))})

	tag := names.NewUnitTag("foo/0")
	tracker, err := secrets.NewSecrets(s.secretsClient, tag, s.stateReadWriter, loggo.GetLogger("test"))
	c.Assert(err, jc.ErrorIsNil)

	err = tracker.SecretsRemoved([]string{"secret:666e2mr0ui3e8a215n4g"})
	c.Assert(err, jc.ErrorIsNil)
}
