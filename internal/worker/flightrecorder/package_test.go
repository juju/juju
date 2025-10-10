// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

//go:generate go run go.uber.org/mock/mockgen -typed -package flightrecorder -destination recorder_mock_test.go github.com/juju/juju/internal/flightrecorder FileRecorder
