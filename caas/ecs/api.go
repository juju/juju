// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/ecs/ecsiface"
	"github.com/juju/errors"

	"github.com/juju/juju/environs/cloudspec"
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

func cloudSpecToAWSConfig(c cloudspec.CloudSpec) (*aws.Config, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	if err := validateCloudCredential(c.Credential); err != nil {
		return nil, errors.Trace(err)
	}
	credentialAttr := c.Credential.Attributes()
	return &aws.Config{
		Retryer: getDefaultRetryer(),
		Region:  aws.String(credentialAttr[credAttrRegionKey]),
		Credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     credentialAttr[credAttrAccessKey],
			SecretAccessKey: credentialAttr[credAttrSecretKey],
		}),
	}, nil
}

func newECSClient(config *aws.Config) (ecsiface.ECSAPI, error) {
	s := session.Must(session.NewSession())
	// Enable request and response logging, but only if TRACE is enabled (as
	// they're probably fairly expensive to produce).
	if logger.IsTraceEnabled() {
		config.Logger = awsLogger{s}
		config.LogLevel = aws.LogLevel(aws.LogDebug | aws.LogDebugWithRequestErrors | aws.LogDebugWithRequestRetries)
	}
	return ecs.New(s, config), nil
}
