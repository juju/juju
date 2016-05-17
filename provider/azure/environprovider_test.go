// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type environProviderSuite struct {
	testing.BaseSuite
	provider environs.EnvironProvider
	requests []*http.Request
	sender   azuretesting.Senders
}

var _ = gc.Suite(&environProviderSuite{})

func (s *environProviderSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.provider, _ = newProviders(c, azure.ProviderConfig{
		Sender:           &s.sender,
		RequestInspector: requestRecorder(&s.requests),
	})
	s.sender = nil
}

func (s *environProviderSuite) TestBootstrapConfigWithInternalConfig(c *gc.C) {
	s.testBootstrapConfigWithInternalConfig(c, "storage-account")
}

func (s *environProviderSuite) testBootstrapConfigWithInternalConfig(c *gc.C, key string) {
	cfg := makeTestModelConfig(c, testing.Attrs{key: "whatever"})
	s.sender = azuretesting.Senders{tokenRefreshSender()}
	_, err := s.provider.BootstrapConfig(environs.BootstrapConfigParams{
		Config:      cfg,
		Credentials: fakeUserPassCredential(),
	})
	c.Check(err, gc.ErrorMatches, fmt.Sprintf(`internal config "%s" must not be specified`, key))
}

func fakeUserPassCredential() cloud.Credential {
	return cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{
			"application-id":       "application-id",
			"subscription-id":      "subscription-id",
			"tenant-id":            "tenant-id",
			"application-password": "application-password",
		},
	)
}

func (s *environProviderSuite) TestBootstrapConfig(c *gc.C) {
	cfg := makeTestModelConfig(c)
	s.sender = azuretesting.Senders{tokenRefreshSender()}
	cfg, err := s.provider.BootstrapConfig(environs.BootstrapConfigParams{
		Config:               cfg,
		CloudRegion:          "westus",
		CloudEndpoint:        "https://api.azurestack.local",
		CloudStorageEndpoint: "https://storage.azurestack.local",
		Credentials:          fakeUserPassCredential(),
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cfg, gc.NotNil)

	attrs := cfg.UnknownAttrs()
	c.Assert(attrs["location"], gc.Equals, "westus")
	c.Assert(attrs["endpoint"], gc.Equals, "https://api.azurestack.local")
	c.Assert(attrs["storage-endpoint"], gc.Equals, "https://storage.azurestack.local")
}

func newProviders(c *gc.C, config azure.ProviderConfig) (environs.EnvironProvider, storage.Provider) {
	if config.NewStorageClient == nil {
		var storage azuretesting.MockStorageClient
		config.NewStorageClient = storage.NewClient
	}
	if config.StorageAccountNameGenerator == nil {
		config.StorageAccountNameGenerator = func() string {
			return fakeStorageAccount
		}
	}
	if config.RetryClock == nil {
		config.RetryClock = testing.NewClock(time.Time{})
	}
	environProvider, storageProvider, err := azure.NewProviders(config)
	c.Assert(err, jc.ErrorIsNil)
	return environProvider, storageProvider
}

func requestRecorder(requests *[]*http.Request) autorest.PrepareDecorator {
	if requests == nil {
		return nil
	}
	return func(p autorest.Preparer) autorest.Preparer {
		return autorest.PreparerFunc(func(req *http.Request) (*http.Request, error) {
			// Save the request body, since it will be consumed.
			reqCopy := *req
			if req.Body != nil {
				var buf bytes.Buffer
				if _, err := buf.ReadFrom(req.Body); err != nil {
					return nil, err
				}
				if err := req.Body.Close(); err != nil {
					return nil, err
				}
				reqCopy.Body = ioutil.NopCloser(&buf)
				req.Body = ioutil.NopCloser(bytes.NewReader(buf.Bytes()))
			}
			*requests = append(*requests, &reqCopy)
			return req, nil
		})
	}
}
