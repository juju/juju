// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	ocicommon "github.com/oracle/oci-go-sdk/v65/common"

	"github.com/juju/juju/internal/provider/oci/common"
	"github.com/juju/juju/internal/testing"
)

type errorsSuite struct {
	testing.BaseSuite
}

func TestErrorsSuite(t *stdtesting.T) { tc.Run(t, &errorsSuite{}) }

type MockServiceError struct {
	ocicommon.ServiceError

	code string
}

func (a MockServiceError) Error() string {
	return fmt.Sprintf("Mocked error %s", a.GetCode())
}

func (a MockServiceError) GetCode() string { return a.code }

func (s *errorsSuite) TestServiceErrorsCanTriggerIsAuthorisationFailure(c *tc.C) {
	err := MockServiceError{code: "NotAuthenticated"}
	result := common.IsAuthorisationFailure(err)
	c.Assert(result, tc.IsTrue)

	err = MockServiceError{code: "InternalServerError"}
	result = common.IsAuthorisationFailure(err)
	c.Assert(result, tc.IsFalse)
}

func (s *errorsSuite) TestUnknownErrorsDoNotTriggerIsAuthorisationFailure(c *tc.C) {
	err1 := errors.New("unknown")
	for _, err := range []error{
		err1,
		errors.Trace(err1),
		errors.Annotate(err1, "really unknown"),
	} {
		c.Assert(common.IsAuthorisationFailure(err), tc.IsFalse)
	}
}

func (s *errorsSuite) TestNilDoesNotTriggerIsAuthorisationFailure(c *tc.C) {
	result := common.IsAuthorisationFailure(nil)
	c.Assert(result, tc.IsFalse)
}
