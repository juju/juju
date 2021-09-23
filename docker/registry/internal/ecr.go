// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/aws/aws-sdk-go/service/ecr/ecriface"
	"github.com/juju/errors"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/tools"
)

// TODO: implement token refresh feature and integrate it into the caasmodelconfigmanager worker!!
// Because the token used for image pulling has to be refreshed every 12hrs.
// https://docs.aws.amazon.com/AmazonECR/latest/userguide/registry_auth.html

type awsLogger struct {
	session *session.Session
}

func (l awsLogger) Log(args ...interface{}) {
	logger.Tracef("awsLogger %p: %s", l.session, fmt.Sprint(args...))
}

func getDefaultRetryer() client.DefaultRetryer {
	return client.DefaultRetryer{
		NumMaxRetries:    10,
		MinRetryDelay:    time.Second,
		MinThrottleDelay: time.Second,
		MaxRetryDelay:    time.Minute,
		MaxThrottleDelay: time.Minute,
	}
}

// GetECRClient is exported for test to patch.
var GetECRClient = getECRClient

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/ecr_mock.go github.com/aws/aws-sdk-go/service/ecr/ecriface ECRAPI
func getECRClient(accessKeyID, secretAccessKey, region string) (ecriface.ECRAPI, error) {
	config := &aws.Config{
		Retryer: getDefaultRetryer(),
		Region:  aws.String(region),
		Credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
		}),
	}
	s := session.Must(session.NewSession())
	// Enable request and response logging, but only if TRACE is enabled (as
	// they're probably fairly expensive to produce).
	if logger.IsTraceEnabled() {
		config.Logger = awsLogger{s}
		config.LogLevel = aws.LogLevel(aws.LogDebug | aws.LogDebugWithRequestErrors | aws.LogDebugWithRequestRetries)
	}
	return ecr.New(s, config), nil
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

func getTokenForElasticContainerRegistry(repoDetails *docker.ImageRepoDetails) (err error) {
	if repoDetails.Region == "" {
		return errors.NewNotValid(nil, "region is required")
	}
	if repoDetails.Username == "" || repoDetails.Password == "" {
		return errors.NewNotValid(nil, fmt.Sprintf("username and password are required for registry %q", repoDetails.Repository))
	}
	c, err := GetECRClient(repoDetails.Username, repoDetails.Password, repoDetails.Region)
	if err != nil {
		return errors.Trace(err)
	}
	result, err := c.GetAuthorizationToken(&ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return errors.Trace(err)
	}
	if len(result.AuthorizationData) > 0 {
		repoDetails.Auth = aws.StringValue(result.AuthorizationData[0].AuthorizationToken)
	}
	if repoDetails.Auth == "" {
		return errors.New(fmt.Sprintf("failed to fetch the authorization token for %q", repoDetails.Repository))
	}
	return nil
}

func elasticContainerRegistryTransport(transport http.RoundTripper, repoDetails *docker.ImageRepoDetails,
) (http.RoundTripper, error) {
	if !repoDetails.TokenAuthConfig.Empty() {
		return nil, errors.New("elastic container registry only supports username and password")
	}
	if repoDetails.BasicAuthConfig.Empty() {
		return nil, errors.NewNotValid(nil, "empty credential for elastic container registry")
	}
	if err := getTokenForElasticContainerRegistry(repoDetails); err != nil {
		return nil, errors.Trace(err)
	}
	return newBasicTransport(transport, "", "", repoDetails.Auth), nil
}

func (c *elasticContainerRegistry) WrapTransport(...TransportWrapper) (err error) {
	if c.client.Transport, err = mergeTransportWrappers(
		c.client.Transport, c.repoDetails,
		elasticContainerRegistryTransport, wrapErrorTransport,
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
	return errors.NotSupportedf("container registry public.ecr.aws")
}
