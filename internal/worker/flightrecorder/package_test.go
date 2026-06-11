// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

//go:generate go run github.com/canonical/gomock/mockgen -package flightrecorder -destination recorder_mock_test.go github.com/juju/juju/internal/worker/flightrecorder FileRecorder
