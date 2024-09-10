// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
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
		name    string
		err     params.Error
		errType error
	}{
		{params.CodeNotFound, params.Error{Code: params.CodeNotFound, Message: "look a NotFound error"}, errors.NotFound},
		{params.CodeUserNotFound, params.Error{Code: params.CodeUserNotFound, Message: "look a UserNotFound error"}, errors.UserNotFound},
		{params.CodeUnauthorized, params.Error{Code: params.CodeUnauthorized, Message: "look a Unauthorized error"}, errors.Unauthorized},
		{params.CodeNotImplemented, params.Error{Code: params.CodeNotImplemented, Message: "look a NotImplemented error"}, errors.NotImplemented},
		{params.CodeAlreadyExists, params.Error{Code: params.CodeAlreadyExists, Message: "look a AlreadyExists error"}, errors.AlreadyExists},
		{params.CodeNotSupported, params.Error{Code: params.CodeNotSupported, Message: "look a NotSupported error"}, errors.NotSupported},
		{params.CodeNotValid, params.Error{Code: params.CodeNotValid, Message: "look a NotValid error"}, errors.NotValid},
		{params.CodeNotProvisioned, params.Error{Code: params.CodeNotProvisioned, Message: "look a NotProvisioned error"}, errors.NotProvisioned},
		{params.CodeNotAssigned, params.Error{Code: params.CodeNotAssigned, Message: "look a NotAssigned error"}, errors.NotAssigned},
		{params.CodeBadRequest, params.Error{Code: params.CodeBadRequest, Message: "look a BadRequest error"}, errors.BadRequest},
		{params.CodeMethodNotAllowed, params.Error{Code: params.CodeMethodNotAllowed, Message: "look a MethodNotAllowed error"}, errors.MethodNotAllowed},
		{params.CodeForbidden, params.Error{Code: params.CodeForbidden, Message: "look a Forbidden error"}, errors.Forbidden},
		{params.CodeQuotaLimitExceeded, params.Error{Code: params.CodeQuotaLimitExceeded, Message: "look a QuotaLimitExceeded error"}, errors.QuotaLimitExceeded},
		{params.CodeNotYetAvailable, params.Error{Code: params.CodeNotYetAvailable, Message: "look a NotYetAvailable error"}, errors.NotYetAvailable},
		{params.CodeModelNotFound, params.Error{Code: params.CodeModelNotFound, Message: "model not found"}, modelerrors.NotFound},
		{params.CodeSecretNotFound, params.Error{Code: params.CodeSecretNotFound, Message: "secret not found"}, secreterrors.SecretNotFound},
		{params.CodeSecretRevisionNotFound, params.Error{Code: params.CodeSecretRevisionNotFound, Message: "secret not found"}, secreterrors.SecretRevisionNotFound},
		{params.CodeSecretConsumerNotFound, params.Error{Code: params.CodeSecretConsumerNotFound, Message: "secret not found"}, secreterrors.SecretConsumerNotFound},
		{params.CodeSecretBackendNotFound, params.Error{Code: params.CodeSecretBackendNotFound, Message: "secret backend not found"}, secretbackenderrors.NotFound},
		{params.CodeSecretBackendAlreadyExists, params.Error{Code: params.CodeSecretBackendAlreadyExists, Message: "secret backend not found"}, secretbackenderrors.AlreadyExists},
		{params.CodeSecretBackendNotSupported, params.Error{Code: params.CodeSecretBackendNotSupported, Message: "secret backend not found"}, secretbackenderrors.NotSupported},
		{params.CodeSecretBackendNotValid, params.Error{Code: params.CodeSecretBackendNotValid, Message: "secret backend not found"}, secretbackenderrors.NotValid},
		{params.CodeSecretBackendForbidden, params.Error{Code: params.CodeSecretBackendForbidden, Message: "secret backend not found"}, secretbackenderrors.Forbidden},
		{params.CodeUnitNotFound, params.Error{Code: params.CodeUnitNotFound, Message: "unit not found"}, applicationerrors.UnitNotFound},
		{params.CodeUnitAlreadyExists, params.Error{Code: params.CodeUnitAlreadyExists, Message: "unit already exists"}, applicationerrors.UnitAlreadyExists},
		{params.CodeApplicationNotFound, params.Error{Code: params.CodeApplicationNotFound, Message: "application not found"}, applicationerrors.ApplicationNotFound},
		{params.CodeScalingStateInconsistent, params.Error{Code: params.CodeScalingStateInconsistent, Message: "scaling state inconsistent"}, applicationerrors.ScalingStateInconsistent},
	}

	for _, v := range tests {
		c.Assert(v.err, gc.Not(jc.ErrorIs), v.errType, gc.Commentf("test %s: params error is not a juju/errors error", v.name))
		c.Assert(params.TranslateWellKnownError(v.err), jc.ErrorIs, v.errType, gc.Commentf("test %s: translated error is a juju/errors error", v.name))
	}
}
