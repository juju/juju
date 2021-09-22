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
)

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

func getTokenForElasticContainerRegistry(repoDetails *docker.ImageRepoDetails) (token string, err error) {
	if repoDetails.Region == "" {
		return "", errors.NewNotValid(nil, "region is required")
	}
	if (repoDetails.Username == "" || repoDetails.Password == "") && repoDetails.Auth != "" {
		if repoDetails.Username, repoDetails.Password, err = unpackAuthToken(repoDetails.Auth); err != nil {
			return "", errors.Annotatef(err, "unpacking auth token for %q", repoDetails.Repository)
		}
	}
	c, err := getECRClient(repoDetails.Username, repoDetails.Password, repoDetails.Region)
	if err != nil {
		return "", errors.Trace(err)
	}
	result, err := c.GetAuthorizationToken(&ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(result.AuthorizationData) > 0 {
		token = aws.StringValue(result.AuthorizationData[0].AuthorizationToken)
	}
	if token == "" {
		return "", errors.NotFoundf("authorization token for %q", repoDetails.Repository)
	}
	return token, nil
}

func elasticContainerRegistryTransport(transport http.RoundTripper, repoDetails *docker.ImageRepoDetails,
) (http.RoundTripper, error) {
	if !repoDetails.TokenAuthConfig.Empty() {
		return nil, errors.New("elastic container registry only supports username and password")
	}
	if repoDetails.BasicAuthConfig.Empty() {
		return nil, errors.NewNotValid(nil, "empty credential for elastic container registry")
	}
	token, err := getTokenForElasticContainerRegistry(repoDetails)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newBasicTransport(transport, "AWS", token, ""), nil
	return transport, nil
}

func (c *elasticContainerRegistry) WrapTransport(...TransportWrapper) (err error) {
	if c.client.Transport, err = mergeTransportWrappers(
		c.client.Transport, c.repoDetails,
		newPrivateOnlyTransport, elasticContainerRegistryTransport, wrapErrorTransport,
	); err != nil {
		return errors.Trace(err)
	}
	return nil
}
