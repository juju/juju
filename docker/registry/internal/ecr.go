// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/smithy-go/logging"
	"github.com/juju/errors"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/tools"
)

// TODO: implement token refresh feature and integrate it into the caasmodelconfigmanager worker!!
// Because the token used for image pulling has to be refreshed every 12hrs.
// https://docs.aws.amazon.com/AmazonECR/latest/userguide/registry_auth.html

// GetECRClient is exported for test to patch.
var GetECRClient = getECRClient

type ecrLogger struct {
	cfg *config.Config
}

func (l ecrLogger) Write(p []byte) (n int, err error) {
	logger.Tracef("ecrLogger %p: %s", l.cfg, p)
	return len(p), nil
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/ecr_mock.go github.com/juju/juju/docker/registry/internal ECRInterface
type ECRInterface interface {
	GetAuthorizationToken(context.Context, *ecr.GetAuthorizationTokenInput, ...func(*ecr.Options)) (*ecr.GetAuthorizationTokenOutput, error)
}

func getECRClient(ctx context.Context, httpClient aws.HTTPClient, accessKeyID, secretAccessKey, region string) (ECRInterface, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithRetryer(func() aws.Retryer {
			return retry.NewStandard()
		}),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Enable request and response logging, but only if TRACE is enabled (as
	// they're probably fairly expensive to produce).
	if logger.IsTraceEnabled() {
		cfg.ClientLogMode = aws.LogRequest | aws.LogResponse | aws.LogRetries
		cfg.Logger = logging.NewStandardLogger(&ecrLogger{})
	}
	cfg.HTTPClient = httpClient
	return ecr.NewFromConfig(cfg), nil
}

type elasticContainerRegistry struct {
	*baseClient
}

func newElasticContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, transport)
	return &elasticContainerRegistry{c}
}

// Match checks if the repository details matches current provider format.
func (c *elasticContainerRegistry) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "amazonaws.com")
}

// APIVersion returns the registry API version to use.
func (c *elasticContainerRegistry) APIVersion() APIVersion {
	// ecr container registry always uses v2.
	return APIVersionV2
}

func (c elasticContainerRegistry) url(pathTemplate string, args ...interface{}) string {
	return commonURLGetter(c.APIVersion(), *c.baseURL, pathTemplate, args...)
}

// DecideBaseURL decides the API url to use.
func (c *elasticContainerRegistry) DecideBaseURL() error {
	return errors.Trace(decideBaseURLCommon(c.APIVersion(), c.repoDetails, c.baseURL))
}

func refreshTokenForElasticContainerRegistry(repoDetails *docker.ImageRepoDetails, httpClient aws.HTTPClient) (err error) {
	if repoDetails.Region == "" {
		return errors.NewNotValid(nil, "region is required")
	}
	if repoDetails.Username == "" || repoDetails.Password == "" {
		return errors.NewNotValid(nil,
			fmt.Sprintf("username and password are required for registry %q", repoDetails.Repository),
		)
	}
	ctx := context.Background()
	c, err := GetECRClient(ctx, httpClient, repoDetails.Username, repoDetails.Password, repoDetails.Region)
	if err != nil {
		return errors.Trace(err)
	}
	result, err := c.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return errors.Trace(err)
	}
	if len(result.AuthorizationData) > 0 {
		data := result.AuthorizationData[0]
		repoDetails.Auth = aws.ToString(data.AuthorizationToken)
		repoDetails.ExpiresAt = data.ExpiresAt
	}
	if repoDetails.Auth == "" {
		return errors.New(fmt.Sprintf("failed to fetch the authorization token for %q", repoDetails.Repository))
	}
	return nil
}

// ShouldRefreshAuth checks if the repoDetails should be refreshed.
func (c *elasticContainerRegistry) ShouldRefreshAuth() bool {
	if c.repoDetails.Auth == "" {
		// auth token is missing, refresh is required.
		return true
	}
	// check if the token has expired or not.
	return c.repoDetails.ExpiresAt == nil || c.repoDetails.ExpiresAt.Before(time.Now())
}

// RefreshAuth refreshes the repoDetails.
func (c *elasticContainerRegistry) RefreshAuth() error {
	return refreshTokenForElasticContainerRegistry(c.repoDetails, c.client)
}

func (c *elasticContainerRegistry) elasticContainerRegistryTransport(
	transport http.RoundTripper, repoDetails *docker.ImageRepoDetails,
) (http.RoundTripper, error) {
	if !repoDetails.TokenAuthConfig.Empty() {
		return nil, errors.New("elastic container registry only supports username and password")
	}
	if repoDetails.BasicAuthConfig.Empty() {
		return nil, errors.NewNotValid(nil, "empty credential for elastic container registry")
	}
	if err := refreshTokenForElasticContainerRegistry(repoDetails, c.client); err != nil {
		return nil, errors.Trace(err)
	}
	return newBasicTransport(transport, "", "", repoDetails.Auth), nil
}

func (c *elasticContainerRegistry) WrapTransport(...TransportWrapper) (err error) {
	if c.client.Transport, err = mergeTransportWrappers(
		c.client.Transport, c.repoDetails,
		c.elasticContainerRegistryTransport, wrapErrorTransport,
	); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Ping pings the ecr endpoint.
func (c elasticContainerRegistry) Ping() error {
	// No ping endpoint available for ecr.
	return nil
}

// Tags fetches tags for an OCI image.
func (c elasticContainerRegistry) Tags(imageName string) (versions tools.Versions, err error) {
	// ecr container registry always uses v2.
	url := c.url("/%s/tags/list", imageName)
	var response tagsResponseV2
	return c.fetchTags(url, &response)
}

type elasticContainerRegistryPublic struct {
	*baseClient
}

func newElasticContainerRegistryPublic(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, transport)
	return &elasticContainerRegistryPublic{c}
}

// Match checks if the repository details matches current provider format.
func (c *elasticContainerRegistryPublic) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "public.ecr.aws")
}

func (c *elasticContainerRegistryPublic) WrapTransport(...TransportWrapper) error {
	return errors.NotSupportedf("container registry %q", c.repoDetails.ServerAddress)
}
