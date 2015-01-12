// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"reflect"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

var ErrUnauthorized = &params.Error{
	Message: "permission denied",
	Code:    params.CodeUnauthorized,
}

func NotFoundError(prefixMessage string) *params.Error {
	return &params.Error{
		Message: fmt.Sprintf("%s not found", prefixMessage),
		Code:    params.CodeNotFound,
	}
}

func NotProvisionedError(machineId string) *params.Error {
	return &params.Error{
		Message: fmt.Sprintf("machine %s not provisioned", machineId),
		Code:    params.CodeNotProvisioned,
	}
}

func NotAssignedError(unitName string) *params.Error {
	return &params.Error{
		Message: fmt.Sprintf("unit %q is not assigned to a machine", unitName),
		Code:    params.CodeNotAssigned,
	}
}

func AlreadyExistsError(what string) *params.Error {
	return &params.Error{
		Message: fmt.Sprintf("%s already exists", what),
		Code:    params.CodeAlreadyExists,
	}
}

func ServerError(message string) *params.Error {
	return &params.Error{
		Message: message,
		Code:    "",
	}
}

func PrefixedError(prefix, message string) *params.Error {
	return ServerError(prefix + message)
}

func AssertNotImplemented(c *gc.C, apiFacade interface{}, methodName string) {
	val := reflect.ValueOf(apiFacade)
	c.Assert(val.IsValid(), jc.IsTrue)
	indir := reflect.Indirect(val)
	c.Assert(indir.IsValid(), jc.IsTrue)
	method := indir.MethodByName(methodName)
	c.Assert(method.IsValid(), jc.IsFalse)
}
