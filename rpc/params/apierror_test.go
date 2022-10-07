// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
)

type errorSuite struct{}

var _ rpc.ErrorCoder = (*params.Error)(nil)

var _ = gc.Suite(&errorSuite{})

func (*errorSuite) TestErrCode(c *gc.C) {
	var err error
	err = &params.Error{Code: params.CodeDead, Message: "brain dead test"}
	c.Check(params.ErrCode(err), gc.Equals, params.CodeDead)

	err = errors.Trace(err)
	c.Check(params.ErrCode(err), gc.Equals, params.CodeDead)
}

func (*errorSuite) TestTranslateWellKnownError(c *gc.C) {
	var tests = []struct {
		name string
		err  params.Error
		test func(err error) bool
	}{
		{params.CodeNotFound, params.Error{Code: params.CodeNotFound, Message: "look a NotFound error"}, errors.IsNotFound},
		{params.CodeUserNotFound, params.Error{Code: params.CodeUserNotFound, Message: "look a UserNotFound error"}, errors.IsUserNotFound},
		{params.CodeUnauthorized, params.Error{Code: params.CodeUnauthorized, Message: "look a Unauthorized error"}, errors.IsUnauthorized},
		{params.CodeNotImplemented, params.Error{Code: params.CodeNotImplemented, Message: "look a NotImplemented error"}, errors.IsNotImplemented},
		{params.CodeAlreadyExists, params.Error{Code: params.CodeAlreadyExists, Message: "look a AlreadyExists error"}, errors.IsAlreadyExists},
		{params.CodeNotSupported, params.Error{Code: params.CodeNotSupported, Message: "look a NotSupported error"}, errors.IsNotSupported},
		{params.CodeNotValid, params.Error{Code: params.CodeNotValid, Message: "look a NotValid error"}, errors.IsNotValid},
		{params.CodeNotProvisioned, params.Error{Code: params.CodeNotProvisioned, Message: "look a NotProvisioned error"}, errors.IsNotProvisioned},
		{params.CodeNotAssigned, params.Error{Code: params.CodeNotAssigned, Message: "look a NotAssigned error"}, errors.IsNotAssigned},
		{params.CodeBadRequest, params.Error{Code: params.CodeBadRequest, Message: "look a BadRequest error"}, errors.IsBadRequest},
		{params.CodeMethodNotAllowed, params.Error{Code: params.CodeMethodNotAllowed, Message: "look a MethodNotAllowed error"}, errors.IsMethodNotAllowed},
		{params.CodeForbidden, params.Error{Code: params.CodeForbidden, Message: "look a Forbidden error"}, errors.IsForbidden},
		{params.CodeQuotaLimitExceeded, params.Error{Code: params.CodeQuotaLimitExceeded, Message: "look a QuotaLimitExceeded error"}, errors.IsQuotaLimitExceeded},
		{params.CodeNotYetAvailable, params.Error{Code: params.CodeNotYetAvailable, Message: "look a NotYetAvailable error"}, errors.IsNotYetAvailable},
	}

	for _, v := range tests {
		c.Assert(v.test(v.err), jc.IsFalse, gc.Commentf("test %s: params error is not a juju/errors error", v.name))
		c.Assert(v.test(params.TranslateWellKnownError(v.err)), jc.IsTrue, gc.Commentf("test %s: translated error is a juju/errors error", v.name))
	}
}
