// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/credentialvalidator"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type CredentialValidatorSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	backend    *testBackend

	api *credentialvalidator.CredentialValidatorAPI
}

var _ = gc.Suite(&CredentialValidatorSuite{})

func (s *CredentialValidatorSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.backend = newMockBackend()

	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	api, err := credentialvalidator.NewCredentialValidatorAPIForTest(s.backend, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *CredentialValidatorSuite) TestModelCredential(c *gc.C) {
	result, err := s.api.ModelCredential()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ModelCredential{
		Model:           names.NewModelTag(modelUUID).String(),
		Exists:          true,
		CloudCredential: credentialTag.String(),
		Valid:           true,
	})
}

func (s *CredentialValidatorSuite) TestModelCredentialNotNeeded(c *gc.C) {
	s.backend.mc.Exists = false
	s.backend.mc.Credential = names.CloudCredentialTag{}
	s.backend.mc.Valid = false
	result, err := s.api.ModelCredential()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ModelCredential{Model: names.NewModelTag(modelUUID).String()})
}

func (s *CredentialValidatorSuite) TestWatchCredential(c *gc.C) {
	result, err := s.api.WatchCredential(params.Entity{credentialTag.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{"1", nil})
	c.Assert(s.resources.Count(), gc.Equals, 1)
}

func (s *CredentialValidatorSuite) TestWatchCredentialNotUsedInThisModel(c *gc.C) {
	s.backend.isUsed = false
	_, err := s.api.WatchCredential(params.Entity{credentialTag.String()})
	c.Assert(err, gc.ErrorMatches, common.ErrPerm.Error())
	c.Assert(s.resources.Count(), gc.Equals, 0)
}

func (s *CredentialValidatorSuite) TestWatchCredentialInvalidTag(c *gc.C) {
	_, err := s.api.WatchCredential(params.Entity{"my-tag"})
	c.Assert(err, gc.ErrorMatches, `"my-tag" is not a valid tag`)
	c.Assert(s.resources.Count(), gc.Equals, 0)
}

func (s *CredentialValidatorSuite) TestInvalidateModelCredential(c *gc.C) {
	result, err := s.api.InvalidateModelCredential(params.InvalidateCredentialArg{"not again"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{})
	s.backend.CheckCalls(c, []testing.StubCall{
		{"InvalidateModelCredential", []interface{}{"not again"}},
	})
}

func (s *CredentialValidatorSuite) TestInvalidateModelCredentialError(c *gc.C) {
	expected := errors.New("boom")
	s.backend.SetErrors(expected)
	result, err := s.api.InvalidateModelCredential(params.InvalidateCredentialArg{"not again"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{Error: common.ServerError(expected)})
	s.backend.CheckCalls(c, []testing.StubCall{
		{"InvalidateModelCredential", []interface{}{"not again"}},
	})
}

func (s *CredentialValidatorSuite) TestWatchModelCredential(c *gc.C) {
	result, err := s.api.WatchModelCredential()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResult{"1", nil})
	c.Assert(s.resources.Count(), gc.Equals, 1)
}

func (s *CredentialValidatorSuite) TestWatchModelCredentialError(c *gc.C) {
	s.backend.SetErrors(errors.New("no nope niet"))
	_, err := s.api.WatchModelCredential()
	c.Assert(err, gc.ErrorMatches, "no nope niet")
	c.Assert(s.resources.Count(), gc.Equals, 0)
}

// modelUUID is the model tag we're using in the tests.
var modelUUID = "01234567-89ab-cdef-0123-456789abcdef"

// credentialTag is the credential tag we're using in the tests.
// needs to fit fmt.Sprintf("%s/%s/%s", cloudName, userName, credentialName)
var credentialTag = names.NewCloudCredentialTag("cloud/user/credential")

func newMockBackend() *testBackend {
	b := &testBackend{
		Stub:   &testing.Stub{},
		isUsed: true,
		mc: &credentialvalidator.ModelCredential{
			Model:      names.NewModelTag(modelUUID),
			Exists:     true,
			Credential: credentialTag,
			Valid:      true,
		},
	}
	return b
}

type testBackend struct {
	*testing.Stub

	mc     *credentialvalidator.ModelCredential
	isUsed bool
}

func (b *testBackend) ModelCredential() (*credentialvalidator.ModelCredential, error) {
	b.AddCall("ModelCredential")
	if err := b.NextErr(); err != nil {
		return nil, err
	}
	return b.mc, nil
}

func (b *testBackend) ModelUsesCredential(tag names.CloudCredentialTag) (bool, error) {
	b.AddCall("ModelUsesCredential", tag)
	if err := b.NextErr(); err != nil {
		return false, err
	}
	return b.isUsed, nil
}

func (b *testBackend) WatchCredential(t names.CloudCredentialTag) state.NotifyWatcher {
	b.AddCall("WatchCredential", t)
	return apiservertesting.NewFakeNotifyWatcher()
}

func (b *testBackend) InvalidateModelCredential(reason string) error {
	b.AddCall("InvalidateModelCredential", reason)
	return b.NextErr()
}

func (b *testBackend) WatchModelCredential() (state.NotifyWatcher, error) {
	b.AddCall("WatchModelCredential")
	if err := b.NextErr(); err != nil {
		return nil, err
	}
	return apiservertesting.NewFakeNotifyWatcher(), nil
}
