// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package s3client -destination session_mock_test.go github.com/juju/juju/internal/s3client Session

func TestSuite(t *stdtesting.T) {
	tc.TestingT(t)
}
