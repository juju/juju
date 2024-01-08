// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package s3client -destination client_mock_test.go github.com/juju/juju/internal/s3client S3Client
//go:generate go run go.uber.org/mock/mockgen -package s3client -destination session_mock_test.go github.com/juju/juju/core/objectstore Session

func TestSuite(t *testing.T) {
	gc.TestingT(t)
}
