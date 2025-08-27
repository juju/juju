// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package common

import (
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	internalstorage "github.com/juju/juju/internal/storage"
	internalprovider "github.com/juju/juju/internal/storage/provider"
)

type storageSuite struct{}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, storageSuite{})
}

// TestCommonIAASProviderTypes tests that the provider types returned from
// [CommonIAASStorageProviderTypes] is what we expect.
//
// If this test fails it means that a breaking change has been introduced. This
// test aims to highlight that breakage.
func (storageSuite) TestCommonIAASProviderTypes(c *tc.C) {
	expectedProviderTypes := []internalstorage.ProviderType{
		internalprovider.TmpfsProviderType,
		internalprovider.RootfsProviderType,
		internalprovider.LoopProviderType,
	}
	c.Check(CommonIAASStorageProviderTypes(), tc.SameContents, expectedProviderTypes)
}

// TestGetCommonIAASStorageProviderNotFound is asserting that when the caller
// asks for a provider type that does not exist they get back an error
// satisfying [coreerrors.NotFound].
func (storageSuite) TestGetCommonIAASStorageProviderNotFound(c *tc.C) {
	notFonndPT := internalstorage.ProviderType("notfound")

	_, err := GetCommonIAASStorageProvider(notFonndPT)
	c.Check(err, tc.ErrorIs, coreerrors.NotFound)
}
