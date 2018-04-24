// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type CredentialValiditySuite struct {
	testing.IsolationSuite

	st *mockState
}

var _ = gc.Suite(&CredentialValiditySuite{})

func (s *CredentialValiditySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.st = &mockState{
		Stub:        &testing.Stub{},
		credentials: map[names.CloudCredentialTag]state.Credential{},
		updated:     map[names.CloudCredentialTag]cloud.Credential{},
	}
}

func (s *CredentialValiditySuite) TestEmptyArgs(c *gc.C) {
	errs, err := credentialcommon.ChangeCloudCredentialsValidity(s.st, params.ValidateCredentialArgs{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.HasLen, 0)
	s.st.CheckCallNames(c, []string{}...) // Nothing was called
}

func (s *CredentialValiditySuite) TestInvalidFlag(c *gc.C) {
	args := []params.ValidateCredentialArg{{CredentialTag: "def-invalid"}}
	errs, err := credentialcommon.ChangeCloudCredentialsValidity(s.st, params.ValidateCredentialArgs{args})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []params.ErrorResult{
		{common.ServerError(errors.Errorf("%q is not a valid tag", "def-invalid"))},
	})
	s.st.CheckCallNames(c, []string{}...) // Nothing was called
}

func (s *CredentialValiditySuite) TestGetCredentialError(c *gc.C) {
	s.st.SetErrors(
		errors.New("boom"),
	)
	args := []params.ValidateCredentialArg{
		{CredentialTag: names.NewCloudCredentialTag("cloud/user/credential").String()},
	}
	errs, err := credentialcommon.ChangeCloudCredentialsValidity(s.st, params.ValidateCredentialArgs{args})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []params.ErrorResult{
		{common.ServerError(errors.New("boom"))},
	})
	s.st.CheckCallNames(c, "CloudCredential")
}

func (s *CredentialValiditySuite) TestChangeCloudCredentialsValidityToValid(c *gc.C) {
	tag := names.NewCloudCredentialTag("cloud/user/credential")
	credOne := statetesting.NewEmptyCredential()
	credOne.Invalid = true
	credOne.InvalidReason = "all for testing"
	c.Assert(credOne.IsValid(), jc.IsFalse)

	s.st.credentials = map[names.CloudCredentialTag]state.Credential{
		tag: credOne,
	}

	errs, err := credentialcommon.ChangeCloudCredentialsValidity(s.st,
		params.ValidateCredentialArgs{
			[]params.ValidateCredentialArg{
				{CredentialTag: tag.String(), Valid: true},
			},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []params.ErrorResult{{}})
	s.st.CheckCallNames(c, "CloudCredential", "UpdateCloudCredential")
	c.Assert(s.st.updated, gc.HasLen, 1)

	sentToState := s.st.updated[tag]
	c.Assert(sentToState.Invalid, jc.IsFalse)
	c.Assert(sentToState.InvalidReason, gc.Equals, "")
}

// TestChangeCloudCredentialsValidityFromValid also tests bulk call.
func (s *CredentialValiditySuite) TestChangeCloudCredentialsValidityFromValid(c *gc.C) {
	tagOne := names.NewCloudCredentialTag("cloud/user/credential")
	credOne := statetesting.NewEmptyCredential()
	c.Assert(credOne.IsValid(), jc.IsTrue)

	tagTwo := names.NewCloudCredentialTag("cloud/user/credentialTwo")
	credTwo := statetesting.NewEmptyCredential()
	c.Assert(credTwo.IsValid(), jc.IsTrue)

	s.st.credentials = map[names.CloudCredentialTag]state.Credential{
		tagOne: credOne,
		tagTwo: credTwo,
	}

	errs, err := credentialcommon.ChangeCloudCredentialsValidity(s.st,
		params.ValidateCredentialArgs{
			[]params.ValidateCredentialArg{
				// valid to start with but invalidate with no reason
				{CredentialTag: tagOne.String(), Valid: false},
				// valid to start with but invalidate with a reason
				{CredentialTag: tagTwo.String(), Valid: false, Reason: "affirmative"},
			},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []params.ErrorResult{{}, {}})
	s.st.CheckCallNames(c, "CloudCredential", "UpdateCloudCredential", "CloudCredential", "UpdateCloudCredential")

	oneSentToState := s.st.updated[tagOne]
	c.Assert(oneSentToState.Invalid, jc.IsTrue)
	c.Assert(oneSentToState.InvalidReason, gc.Equals, "")

	twoSentToState := s.st.updated[tagTwo]
	c.Assert(twoSentToState.Invalid, jc.IsTrue)
	c.Assert(twoSentToState.InvalidReason, gc.Equals, "affirmative")
}

type mockState struct {
	*testing.Stub
	credentials map[names.CloudCredentialTag]state.Credential
	updated     map[names.CloudCredentialTag]cloud.Credential
}

func (s *mockState) CloudCredential(tag names.CloudCredentialTag) (state.Credential, error) {
	s.MethodCall(s, "CloudCredential", tag)
	if err := s.NextErr(); err != nil {
		return state.Credential{}, err
	}
	return s.credentials[tag], nil
}

func (s *mockState) UpdateCloudCredential(tag names.CloudCredentialTag, credential cloud.Credential) error {
	s.MethodCall(s, "UpdateCloudCredential", tag, credential)
	if err := s.NextErr(); err != nil {
		return err
	}
	s.updated[tag] = credential
	return nil
}
