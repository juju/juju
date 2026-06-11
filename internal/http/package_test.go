// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

//go:generate go run github.com/canonical/gomock/mockgen -package http -destination client_mock_test.go github.com/juju/juju/internal/http RequestRecorder,RoundTripper
//go:generate go run github.com/canonical/gomock/mockgen -package http -destination http_mock_test.go github.com/juju/juju/core/http HTTPClient
//go:generate go run github.com/canonical/gomock/mockgen -package http -destination clock_mock_test.go github.com/juju/clock Clock
