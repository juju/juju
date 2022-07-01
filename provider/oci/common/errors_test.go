// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	ocicommon "github.com/oracle/oci-go-sdk/v47/common"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/v2/provider/oci/common"
	"github.com/juju/juju/v2/testing"
	jc "github.com/juju/testing/checkers"
)

type errorsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&errorsSuite{})

type MockServiceError struct {
	ocicommon.ServiceError

	code string
}

func (a MockServiceError) Error() string {
	return fmt.Sprintf("Mocked error %s", a.GetCode())
}

func (a MockServiceError) GetCode() string { return a.code }

func (s *errorsSuite) TestServiceErrorsCanTriggerIsAuthorisationFailure(c *gc.C) {
	err := MockServiceError{code: "NotAuthenticated"}
	result := common.IsAuthorisationFailure(err)
	c.Assert(result, jc.IsTrue)

	err = MockServiceError{code: "InternalServerError"}
	result = common.IsAuthorisationFailure(err)
	c.Assert(result, jc.IsFalse)
}

func (s *errorsSuite) TestUnknownErrorsDoNotTriggerIsAuthorisationFailure(c *gc.C) {
	err1 := errors.New("unknown")
	for _, err := range []error{
		err1,
		errors.Trace(err1),
		errors.Annotate(err1, "really unknown"),
	} {
		c.Assert(common.IsAuthorisationFailure(err), jc.IsFalse)
	}
}

func (s *errorsSuite) TestNilDoesNotTriggerIsAuthorisationFailure(c *gc.C) {
	result := common.IsAuthorisationFailure(nil)
	c.Assert(result, jc.IsFalse)
}
