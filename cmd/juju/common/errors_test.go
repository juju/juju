// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"bytes"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/juju/juju/cmd/juju/common"
)

type errorsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&errorsSuite{})

func (s *errorsSuite) TestTermsRequiredPassThru(c *gc.C) {
	err := errors.New("nothing about terms")
	c.Assert(err, gc.Equals, common.MaybeTermsAgreementError(err))
}

func (s *errorsSuite) TestBakeryNonTerms(c *gc.C) {
	err := &httpbakery.DischargeError{Reason: &httpbakery.Error{
		Code: "bad cookie",
	}}
	c.Assert(err, gc.Equals, common.MaybeTermsAgreementError(err))
	err = &httpbakery.DischargeError{Reason: &httpbakery.Error{
		Code:    "term agreement required",
		Message: "but terms not specified in message",
	}}
	c.Assert(err, gc.Equals, common.MaybeTermsAgreementError(err))
}

func (s *errorsSuite) TestSingleTermRequired(c *gc.C) {
	err := &httpbakery.DischargeError{Reason: &httpbakery.Error{
		Code:    "term agreement required",
		Message: "term agreement required: foo/1",
	}}
	termErr, ok := common.MaybeTermsAgreementError(err).(*common.TermsRequiredError)
	c.Assert(ok, jc.IsTrue, gc.Commentf("failed to match common.TermsRequiredError"))
	c.Assert(termErr, gc.ErrorMatches, `.*please agree to terms "foo/1".*`)
	c.Assert(termErr.UserErr(), gc.ErrorMatches,
		`.*Declined: some terms require agreement. Try: "juju agree foo/1".*`)
}

func (s *errorsSuite) TestMultipleTermsRequired(c *gc.C) {
	err := &httpbakery.DischargeError{Reason: &httpbakery.Error{
		Code:    "term agreement required",
		Message: "term agreement required: foo/1 bar/2",
	}}
	termErr, ok := common.MaybeTermsAgreementError(err).(*common.TermsRequiredError)
	c.Assert(ok, jc.IsTrue, gc.Commentf("failed to match common.TermsRequiredError"))
	c.Assert(termErr, gc.ErrorMatches, `.*please agree to terms "foo/1 bar/2".*`)
	c.Assert(termErr.UserErr(), gc.ErrorMatches,
		`.*Declined: some terms require agreement. Try: "juju agree foo/1 bar/2".*`)
}

func (s *errorsSuite) TestPermissionsMessage(c *gc.C) {
	var buf bytes.Buffer
	common.PermissionsMessage(&buf, "bork")
	c.Assert(buf.String(), jc.Contains, `You do not have permission to bork.`)
	buf.Reset()
	common.PermissionsMessage(&buf, "")
	c.Assert(buf.String(), jc.Contains, `You do not have permission to complete this operation.`)
}
