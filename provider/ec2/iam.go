// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	stdcontext "context"
	stderrors "errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
)

// instanceProfileClient is a subset interface of the ec2 client for attaching
// Instance Profiles to ec2 machines.
type instanceProfileClient interface {
	AssociateIamInstanceProfile(stdcontext.Context, *ec2.AssociateIamInstanceProfileInput, ...func(*ec2.Options)) (*ec2.AssociateIamInstanceProfileOutput, error)
}

// IAMClient is a subset interface of the AWS IAM client. This interface aims
// to define the small set of what Juju's needs from the larger client.
type IAMClient interface {
	CreateInstanceProfile(stdcontext.Context, *iam.CreateInstanceProfileInput, ...func(*iam.Options)) (*iam.CreateInstanceProfileOutput, error)
	GetInstanceProfile(stdcontext.Context, *iam.GetInstanceProfileInput, ...func(*iam.Options)) (*iam.GetInstanceProfileOutput, error)
}

// IAMClientFunc defines a type that can generate an AWS IAMClient from a
// provided cloudspec.
type IAMClientFunc = func(stdcontext.Context, cloudspec.CloudSpec, ...ClientOption) (IAMClient, error)

const (
	setProfileDelay      = time.Second * 5
	setProfileMaxAttempt = 5
)

// iamClientFunc implements the IAMClientFunc type and is used internally by
// Juju for creating an IAM client.
func iamClientFunc(
	ctx stdcontext.Context,
	spec cloudspec.CloudSpec,
	clientOptions ...ClientOption,
) (IAMClient, error) {
	cfg, err := configFromCloudSpec(ctx, spec, clientOptions...)
	if err != nil {
		return nil, errors.Annotate(err, "building aws config from cloudspec")
	}
	return iam.NewFromConfig(cfg), nil
}

// ensureControllerInstanceProfile ensures that a controller Instance Profile
// has been created for the supplied controller name in the specified AWS cloud.
func ensureControllerInstanceProfile(
	ctx stdcontext.Context,
	client IAMClient,
	controllerName string,
) (*iamtypes.InstanceProfile, error) {
	profileName := fmt.Sprintf("juju-controller-%s", controllerName)
	res, err := client.CreateInstanceProfile(ctx, &iam.CreateInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
	})
	if err != nil {
		var alreadyExistsErr *iamtypes.EntityAlreadyExistsException
		if stderrors.As(err, &alreadyExistsErr) {
			// Instance Profile already exists so we don't need todo anything. Let just find it
			return findInstanceProfileFromName(ctx, client, profileName)
		}
		// Some other error that we can't recover from.
		return nil, errors.Annotate(err, "creating controller instance profile")
	}
	return res.InstanceProfile, nil
}

// findInstanceProfileForName is responsible for finding the concrete instance
// profile for a supplied name. This is used to subsequently fetch the ARN of
// the InstanceProfile.
func findInstanceProfileFromName(
	ctx stdcontext.Context,
	client IAMClient,
	name string,
) (*iamtypes.InstanceProfile, error) {
	res, err := client.GetInstanceProfile(ctx, &iam.GetInstanceProfileInput{
		InstanceProfileName: &name,
	})

	if err != nil {
		var opHTTPErr *awshttp.ResponseError
		if stderrors.As(err, &opHTTPErr) && opHTTPErr.HTTPStatusCode() == http.StatusNotFound {
			return nil, errors.NotFoundf("instance profile %q not found", name)
		}
		return nil, errors.Annotatef(err, "finding instance profile for name %s", name)
	}

	return res.InstanceProfile, nil
}

// setInstanceProfileWithWait sets the instnace profile for a given instance
// blocking until the instance is in a running state where the profile can be
// applied. Attaching Instance Profiles is eventually consistent in AWS and
// the completion of this function does not garuntee that the Instance Profile
// is active yet.
func setInstanceProfileWithWait(
	ctx context.ProviderCallContext,
	client instanceProfileClient,
	profile *iamtypes.InstanceProfile,
	inst instances.Instance,
	instLister environs.InstanceLister,
) error {
	return retry.Call(retry.CallArgs{
		Attempts: setProfileMaxAttempt,
		Delay:    setProfileDelay,
		Func: func() error {
			return setInstanceProfile(ctx, client, profile, inst, instLister)
		},
		IsFatalError: func(err error) bool {
			return !errors.IsNotProvisioned(err)
		},
		BackoffFunc: retry.DoubleDelay,
		Clock:       clock.WallClock,
		Stop:        ctx.Done(),
	})

}

// setInstanceProfile sets the instance profile for a given instance. This
// function first checks to see that the supplied instance is in a running
// state first otherwise a Juju NotProvisioned error returned. Use
// setInstanceProfileWithWait to block on the instance status being running.
func setInstanceProfile(
	ctx context.ProviderCallContext,
	client instanceProfileClient,
	profile *iamtypes.InstanceProfile,
	inst instances.Instance,
	instLister environs.InstanceLister,
) error {
	rInst, err := instLister.Instances(ctx, []instance.Id{inst.Id()})
	if err != nil {
		return errors.Annotatef(err, "listing instance with id %s", inst.Id())
	}
	if len(rInst) != 1 {
		return errors.Errorf("expected 1 instance for id %s got %d", inst.Id(), len(rInst))
	}

	if rInst[0].Status(ctx).Status != status.Running {
		return errors.NotProvisionedf("instance %s is not running", inst.Id())
	}

	instanceProfileInput := ec2.AssociateIamInstanceProfileInput{
		IamInstanceProfile: &ec2types.IamInstanceProfileSpecification{
			Arn:  profile.Arn,
			Name: profile.InstanceProfileName,
		},
		InstanceId: aws.String(string(inst.Id())),
	}

	_, err = client.AssociateIamInstanceProfile(ctx, &instanceProfileInput)
	if err != nil {
		return errors.Annotatef(
			err,
			"attaching instance profile %s to instance %s",
			*profile.InstanceProfileName,
			inst.Id())
	}

	return nil
}
