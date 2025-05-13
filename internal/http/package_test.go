// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package http -destination client_mock_test.go github.com/juju/juju/internal/http RequestRecorder,RoundTripper
//go:generate go run go.uber.org/mock/mockgen -typed -package http -destination http_mock_test.go github.com/juju/juju/core/http HTTPClient
//go:generate go run go.uber.org/mock/mockgen -typed -package http -destination clock_mock_test.go github.com/juju/clock Clock

func Test(t *testing.T) {
	tc.TestingT(t)
}
