// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/Azure/go-autorest/autorest"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
	"github.com/juju/juju/testing"
)

type environProviderSuite struct {
	testing.BaseSuite
	provider environs.EnvironProvider
	spec     environs.CloudSpec
	requests []*http.Request
	sender   azuretesting.Senders
}

var _ = gc.Suite(&environProviderSuite{})

func (s *environProviderSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.provider = newProvider(c, azure.ProviderConfig{
		Sender:           &s.sender,
		RequestInspector: requestRecorder(&s.requests),
	})
	s.spec = environs.CloudSpec{
		Type:             "azure",
		Name:             "azure",
		Region:           "westus",
		Endpoint:         "https://api.azurestack.local",
		IdentityEndpoint: "https://login.azurestack.local",
		StorageEndpoint:  "https://storage.azurestack.local",
		Credential:       fakeUserPassCredential(),
	}
	s.sender = nil
}

func fakeUserPassCredential() *cloud.Credential {
	cred := cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{
			"application-id":       fakeApplicationId,
			"subscription-id":      fakeSubscriptionId,
			"tenant-id":            fakeTenantId,
			"application-password": "opensezme",
		},
	)
	return &cred
}

func (s *environProviderSuite) TestPrepareConfig(c *gc.C) {
	cfg := makeTestModelConfig(c)
	s.sender = azuretesting.Senders{tokenRefreshSender()}
	cfg, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud:  s.spec,
		Config: cfg,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg, gc.NotNil)
}

func (s *environProviderSuite) TestOpen(c *gc.C) {
	env, err := s.provider.Open(environs.OpenParams{
		Cloud:  s.spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
}

func (s *environProviderSuite) TestOpenMissingCredential(c *gc.C) {
	s.spec.Credential = nil
	s.testOpenError(c, s.spec, `validating cloud spec: missing credential not valid`)
}

func (s *environProviderSuite) TestOpenUnsupportedCredential(c *gc.C) {
	credential := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{})
	s.spec.Credential = &credential
	s.testOpenError(c, s.spec, `validating cloud spec: "oauth1" auth-type not supported`)
}

func (s *environProviderSuite) testOpenError(c *gc.C, spec environs.CloudSpec, expect string) {
	_, err := s.provider.Open(environs.OpenParams{
		Cloud:  spec,
		Config: makeTestModelConfig(c),
	})
	c.Assert(err, gc.ErrorMatches, expect)
}

func newProvider(c *gc.C, config azure.ProviderConfig) environs.EnvironProvider {
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
		config.RetryClock = jujutesting.NewClock(time.Time{})
	}
	environProvider, err := azure.NewProvider(config)
	c.Assert(err, jc.ErrorIsNil)
	return environProvider
}

func requestRecorder(requests *[]*http.Request) autorest.PrepareDecorator {
	if requests == nil {
		return nil
	}
	var mu sync.Mutex
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
			mu.Lock()
			*requests = append(*requests, &reqCopy)
			mu.Unlock()
			return req, nil
		})
	}
}
