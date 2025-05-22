// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

//go:generate go run go.uber.org/mock/mockgen -typed -package s3client -destination session_mock_test.go github.com/juju/juju/internal/s3client Session
