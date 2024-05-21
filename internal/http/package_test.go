// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package http -destination client_mock_test.go github.com/juju/juju/internal/http HTTPClient,RequestRecorder,Logger
//go:generate go run go.uber.org/mock/mockgen -typed -package http -destination http_mock_test.go github.com/juju/juju/internal/http RoundTripper
//go:generate go run go.uber.org/mock/mockgen -typed -package http -destination clock_mock_test.go github.com/juju/clock Clock

func Test(t *testing.T) {
	gc.TestingT(t)
}
