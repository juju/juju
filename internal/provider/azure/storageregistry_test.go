// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/azure"
	"github.com/juju/juju/internal/provider/azure/internal/azuretesting"
	"github.com/juju/juju/internal/storage"
)

// storageRegistrySuite is a testing suite for asserting the behaviour of the
// storage.ProviderRegistry implementation on the azure environ.
type storageRegistrySuite struct {
}

func TestStorageRegistrySuite(t *testing.T) {
	tc.Run(t, &storageRegistrySuite{})
}

func (s *storageRegistrySuite) TestRecommendedPoolForKind(c *tc.C) {
	credentialInvalidator := azure.CredentialInvalidator(func(context.Context, environs.CredentialInvalidReason) error {
		return nil
	})
	var (
		sender   azuretesting.Senders
		requests []*http.Request
	)
	envProvider := newProvider(c, azure.ProviderConfig{
		Sender:           &sender,
		RequestInspector: &azuretesting.RequestRecorderPolicy{Requests: &requests},
		CreateTokenCredential: func(appId, appPassword, tenantID string, opts azcore.ClientOptions) (azcore.TokenCredential, error) {
			return &azuretesting.FakeCredential{}, nil
		},
	})
	env := openEnviron(c, envProvider, credentialInvalidator, &sender)

	pool := env.RecommendedPoolForKind(storage.StorageKindBlock)
	c.Check(pool.Name(), tc.Equals, "azure")
	c.Check(pool.Provider().String(), tc.Equals, "azure")

	pool = env.RecommendedPoolForKind(storage.StorageKindFilesystem)
	c.Check(pool.Name(), tc.Equals, "azure")
	c.Check(pool.Provider().String(), tc.Equals, "azure")
}
