// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender

import (
	stderrors "errors"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
)

type logDeliveryErrorSuite struct{}

func TestLogDeliveryErrorSuite(t *testing.T) {
	tc.Run(t, &logDeliveryErrorSuite{})
}

func (s *logDeliveryErrorSuite) TestLogSinkUnavailableError(c *tc.C) {
	err := errors.WithType(
		stderrors.New("cannot connect to /logsink: server returned HTTP status 503"),
		api.HTTPStatusServiceUnavailable,
	)

	c.Check(isLogSinkUnavailableError(err), tc.IsTrue)
	c.Check(isLogSinkUnavailableError(stderrors.New(
		"cannot connect to /logsink: server returned HTTP status 503",
	)), tc.IsFalse)
	c.Check(isLogSinkUnavailableError(nil), tc.IsFalse)
}
