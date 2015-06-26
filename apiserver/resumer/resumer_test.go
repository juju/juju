// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer_test

import (
	"errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/resumer"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
)

type ResumerSuite struct {
	coretesting.BaseSuite

	st         *mockState
	api        *resumer.ResumerAPI
	authoriser apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&ResumerSuite{})

func (s *ResumerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.authoriser = apiservertesting.FakeAuthorizer{
		EnvironManager: true,
	}
	s.st = &mockState{&testing.Stub{}}
	resumer.PatchState(s, s.st)
	var err error
	s.api, err = resumer.NewResumerAPI(nil, nil, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ResumerSuite) TestNewResumerAPIRequiresEnvironManager(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.EnvironManager = false
	api, err := resumer.NewResumerAPI(nil, nil, anAuthoriser)
	c.Assert(api, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ResumerSuite) TestResumeTransactionsFailure(c *gc.C) {
	s.st.SetErrors(errors.New("boom!"))

	err := s.api.ResumeTransactions()
	c.Assert(err, gc.ErrorMatches, "boom!")
	s.st.CheckCalls(c, []testing.StubCall{{
		FuncName: "ResumeTransactions",
		Args:     nil,
	}})
}

func (s *ResumerSuite) TestResumeTransactionsSuccess(c *gc.C) {
	err := s.api.ResumeTransactions()
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCalls(c, []testing.StubCall{{
		FuncName: "ResumeTransactions",
		Args:     nil,
	}})
}

type mockState struct {
	*testing.Stub
}

func (st *mockState) ResumeTransactions() error {
	st.MethodCall(st, "ResumeTransactions")
	return st.NextErr()
}
