// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki_test

import (
	"os"
	"testing"

	"github.com/juju/juju/internal/pki"
	pki_test "github.com/juju/juju/internal/pki/test"
)

func TestMain(m *testing.M) {
	if pki_test.OriginalDefaultKeyProfile == nil {
		panic("pki_test.OriginalDefaultKeyProfile not set")
	}
	// Restore the correct key profile.
	pki.DefaultKeyProfile = pki_test.OriginalDefaultKeyProfile
	os.Exit(m.Run())
}
