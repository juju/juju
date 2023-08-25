// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package muxhttpserver_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/pki"
	pki_test "github.com/juju/juju/internal/pki/test"
)

func TestSuite(t *testing.T) { gc.TestingT(t) }

func init() {
	// Use full strength key profile
	pki.DefaultKeyProfile = pki_test.OriginalDefaultKeyProfile
}
