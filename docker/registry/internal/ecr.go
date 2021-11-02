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

// The ECR auth token expires after 12 hours.
// We refresh the token 5 mins before it's expired.
// https://docs.aws.amazon.com/AmazonECR/latest/userguide/registry_auth.html
const advanceExpiry = 5 * time.Minute

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

func getECRClient(ctx context.Context, accessKeyID, secretAccessKey, region string) (ECRInterface, error) {
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
	return ecr.NewFromConfig(cfg), nil
}

type elasticContainerRegistry struct {
	*baseClient
	ECRClientFunc func(ctx context.Context, accessKeyID, secretAccessKey, region string) (ECRInterface, error)
}

func newElasticContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	return newElasticContainerRegistryForTest(repoDetails, transport, getECRClient)
}

func newElasticContainerRegistryForTest(
	repoDetails docker.ImageRepoDetails, transport http.RoundTripper,
	ECRClientFunc func(ctx context.Context, accessKeyID, secretAccessKey, region string) (ECRInterface, error),
) RegistryInternal {
	c := newBase(repoDetails, transport, normalizeRepoDetailsElasticContainerRegistry)
	return &elasticContainerRegistry{baseClient: c, ECRClientFunc: ECRClientFunc}
}

func normalizeRepoDetailsElasticContainerRegistry(repoDetails *docker.ImageRepoDetails) {
	if repoDetails.ServerAddress == "" {
		repoDetails.ServerAddress = repoDetails.Repository
	}
}

// Match checks if the repository details matches current provider format.
func (c *elasticContainerRegistry) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "amazonaws.com")
}

func (c *elasticContainerRegistry) refreshTokenForElasticContainerRegistry(imageRepo *docker.ImageRepoDetails) (err error) {
	if imageRepo.Region == "" {
		return errors.NewNotValid(nil, "region is required")
	}
	if imageRepo.Username == "" || imageRepo.Password == "" {
		return errors.NewNotValid(nil,
			fmt.Sprintf("username and password are required for registry %q", imageRepo.Repository),
		)
	}
	ctx := context.Background()
	client, err := c.ECRClientFunc(ctx, imageRepo.Username, imageRepo.Password, imageRepo.Region)
	if err != nil {
		return errors.Trace(err)
	}
	result, err := client.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return errors.Trace(err)
	}
	if len(result.AuthorizationData) > 0 {
		data := result.AuthorizationData[0]
		imageRepo.Auth = docker.NewToken(aws.ToString(data.AuthorizationToken))
		if !imageRepo.Auth.Empty() {
			imageRepo.Auth.ExpiresAt = data.ExpiresAt
		}
	}
	if imageRepo.Auth.Empty() {
		return errors.New(fmt.Sprintf("failed to fetch the authorization token for %q", imageRepo.Repository))
	}
	return nil
}

// ShouldRefreshAuth checks if the repoDetails should be refreshed.
func (c *elasticContainerRegistry) ShouldRefreshAuth() (bool, *time.Duration) {
	if c.repoDetails.Auth.Empty() || c.repoDetails.Auth.ExpiresAt == nil {
		return true, nil
	}
	d := time.Until(*c.repoDetails.Auth.ExpiresAt)
	if d <= advanceExpiry {
		return true, nil
	}
	nextCheckDuration := d - advanceExpiry
	return false, &nextCheckDuration
}

// RefreshAuth refreshes the repoDetails.
func (c *elasticContainerRegistry) RefreshAuth() error {
	return c.refreshTokenForElasticContainerRegistry(c.repoDetails)
}

func (c *elasticContainerRegistry) elasticContainerRegistryTransport(
	transport http.RoundTripper, repoDetails *docker.ImageRepoDetails,
) (http.RoundTripper, error) {
	if repoDetails.BasicAuthConfig.Empty() {
		return nil, errors.NewNotValid(nil, "empty credential for elastic container registry")
	}
	if err := c.refreshTokenForElasticContainerRegistry(repoDetails); err != nil {
		return nil, errors.Trace(err)
	}
	if repoDetails.Auth.Empty() {
		return nil, errors.NewNotValid(nil, "empty identity token for elastic container registry")
	}
	return newBasicTransport(transport, "", "", repoDetails.Auth.Value), nil
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
	url := c.url("/%s/tags/list", imageName)
	var response tagsResponseV2
	return c.fetchTags(url, &response)
}

// GetArchitecture returns the archtecture of the image for the specified tag.
func (c elasticContainerRegistry) GetArchitecture(imageName, tag string) (string, error) {
	return getArchitecture(imageName, tag, c)
}

// GetManifests returns the manifests of the image for the specified tag.
func (c elasticContainerRegistry) GetManifests(imageName, tag string) (*ManifestsResult, error) {
	url := c.url("/%s/manifests/%s", imageName, tag)
	return c.GetManifestsCommon(url)
}

// GetBlobs gets the archtecture of the image for the specified tag via blobs API.
func (c elasticContainerRegistry) GetBlobs(imageName, digest string) (*BlobsResponse, error) {
	url := c.url("/%s/blobs/%s", imageName, digest)
	return c.GetBlobsCommon(url)
}

type elasticContainerRegistryPublic struct {
	*baseClient
}

func newElasticContainerRegistryPublic(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, transport, normalizeRepoDetailsCommon)
	return &elasticContainerRegistryPublic{c}
}

// Match checks if the repository details matches current provider format.
func (c *elasticContainerRegistryPublic) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "public.ecr.aws")
}

func (c *elasticContainerRegistryPublic) WrapTransport(...TransportWrapper) error {
	// `/manifests` and `/blobs` API work.
	// but `/tags/list` does not work. - https://github.com/aws/containers-roadmap/issues/1262
	return errors.NotSupportedf("container registry %q", c.repoDetails.ServerAddress)
}
