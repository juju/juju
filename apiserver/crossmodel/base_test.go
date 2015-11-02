// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
)

type baseCrossmodelSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer testing.FakeAuthorizer

	api   *crossmodel.API
	state *mockState

	calls []string
}

func (s *baseCrossmodelSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{names.NewUserTag("testuser"), true}
	s.calls = []string{}
	s.state = s.constructState()

	var err error
	s.api, err = crossmodel.CreateAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *baseCrossmodelSuite) assertCalls(c *gc.C, expectedCalls []string) {
	c.Assert(s.calls, jc.SameContents, expectedCalls)
}

const (
	offerCall = "offer"
)

func (s *baseCrossmodelSuite) constructState() *mockState {
	return &mockState{
		offer: func(one params.CrossModelOffer) error {
			s.calls = append(s.calls, offerCall)
			return nil
		},
	}
}

type mockState struct {
	offer func(one params.CrossModelOffer) error
}

func (st *mockState) Offer(o params.CrossModelOffer) error {
	return st.offer(o)
}
