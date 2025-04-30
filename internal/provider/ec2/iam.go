// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"
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
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/tags"
)

// instanceProfileClient is a subset interface of the ec2 client for attaching
// Instance Profiles to ec2 machines.
type instanceProfileClient interface {
	AssociateIamInstanceProfile(context.Context, *ec2.AssociateIamInstanceProfileInput, ...func(*ec2.Options)) (*ec2.AssociateIamInstanceProfileOutput, error)
	DescribeIamInstanceProfileAssociations(context.Context, *ec2.DescribeIamInstanceProfileAssociationsInput, ...func(*ec2.Options)) (*ec2.DescribeIamInstanceProfileAssociationsOutput, error)
}

// IAMClient is a subset interface of the AWS IAM client. This interface aims
// to define the small set of what Juju's needs from the larger client.
type IAMClient interface {
	// STOP!!
	// Are you about to add a new function to this interface?
	// If so please make sure you update Juju permission policy on discourse
	// here https://discourse.charmhub.io/t/juju-aws-permissions/5307
	// We must keep this policy inline with our usage for operators that are
	// using very strict permissions for Juju.
	//
	// You must also update the controllerRolePolicy document found in
	// iam_docs.go.
	AddRoleToInstanceProfile(context.Context, *iam.AddRoleToInstanceProfileInput, ...func(*iam.Options)) (*iam.AddRoleToInstanceProfileOutput, error)
	CreateInstanceProfile(context.Context, *iam.CreateInstanceProfileInput, ...func(*iam.Options)) (*iam.CreateInstanceProfileOutput, error)
	CreateRole(context.Context, *iam.CreateRoleInput, ...func(*iam.Options)) (*iam.CreateRoleOutput, error)
	DeleteInstanceProfile(context.Context, *iam.DeleteInstanceProfileInput, ...func(*iam.Options)) (*iam.DeleteInstanceProfileOutput, error)
	DeleteRole(context.Context, *iam.DeleteRoleInput, ...func(*iam.Options)) (*iam.DeleteRoleOutput, error)
	DeleteRolePolicy(context.Context, *iam.DeleteRolePolicyInput, ...func(*iam.Options)) (*iam.DeleteRolePolicyOutput, error)
	GetInstanceProfile(context.Context, *iam.GetInstanceProfileInput, ...func(*iam.Options)) (*iam.GetInstanceProfileOutput, error)
	GetRole(context.Context, *iam.GetRoleInput, ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	ListInstanceProfiles(context.Context, *iam.ListInstanceProfilesInput, ...func(*iam.Options)) (*iam.ListInstanceProfilesOutput, error)
	ListRolePolicies(context.Context, *iam.ListRolePoliciesInput, ...func(*iam.Options)) (*iam.ListRolePoliciesOutput, error)
	ListRoles(context.Context, *iam.ListRolesInput, ...func(*iam.Options)) (*iam.ListRolesOutput, error)
	PutRolePolicy(context.Context, *iam.PutRolePolicyInput, ...func(*iam.Options)) (*iam.PutRolePolicyOutput, error)
	RemoveRoleFromInstanceProfile(context.Context, *iam.RemoveRoleFromInstanceProfileInput, ...func(*iam.Options)) (*iam.RemoveRoleFromInstanceProfileOutput, error)
}

// IAMClientFunc defines a type that can generate an AWS IAMClient from a
// provided cloudspec.
type IAMClientFunc = func(context.Context, cloudspec.CloudSpec, ...ClientOption) (IAMClient, error)

const (
	// setProfileAssociationDelay is the delay between retry attempts when.
	setProfileAssociationDelay = time.Second * 15

	// setProfileAssociationMaxAttempt is the maxium number of attempts before
	// giving up on iam profile association.
	setProfileAssociationMaxAttempt = 5

	// setProfileDelay is the delay between retry attempts when setting an
	// instances iam profile.
	setProfileDelay = time.Second * 5

	// setProfileMaxAttempt is the maxium number of attempts before giving up
	// on setting an instances iam profile.
	setProfileMaxAttempt = 5
)

// iamClientFunc implements the IAMClientFunc type and is used internally by
// Juju for creating an IAM client.
func iamClientFunc(
	ctx context.Context,
	spec cloudspec.CloudSpec,
	clientOptions ...ClientOption,
) (IAMClient, error) {
	cfg, err := configFromCloudSpec(ctx, spec, clientOptions...)
	if err != nil {
		return nil, errors.Annotate(err, "building aws config from cloudspec")
	}
	return iam.NewFromConfig(cfg), nil
}

// controllerPath returns an AWS path to use for IAM resources based on the
// controller UUID
func controllerPath(controllerUUID string) string {
	return fmt.Sprintf("/juju/controller/%s/", controllerUUID)
}

// deleteInstanceProfile is a convience method for removing instance profile by
// first detaching all roles from the profile then deleting.
func deleteInstanceProfile(
	ctx context.Context,
	client IAMClient,
	instanceProfile iamtypes.InstanceProfile,
) error {
	for _, role := range instanceProfile.Roles {
		_, err := client.RemoveRoleFromInstanceProfile(ctx, &iam.RemoveRoleFromInstanceProfileInput{
			InstanceProfileName: instanceProfile.InstanceProfileName,
			RoleName:            role.RoleName,
		})

		if err != nil {
			return errors.Annotatef(err, "removing role %q from instance profile", *role.RoleName)
		}
	}

	_, err := client.DeleteInstanceProfile(ctx, &iam.DeleteInstanceProfileInput{
		InstanceProfileName: instanceProfile.InstanceProfileName,
	})

	return errors.Trace(err)
}

// deleteRole is a convience method for delete a role and it's associated
// inline policies.
func deleteRole(
	ctx context.Context,
	client IAMClient,
	roleName string,
) error {

	var (
		inlinePolicyNames = []string{}
		marker            *string
	)
	for {
		res, err := client.ListRolePolicies(ctx, &iam.ListRolePoliciesInput{
			Marker:   marker,
			RoleName: aws.String(roleName),
		})

		if err != nil {
			return errors.Annotatef(err, "listing inline policies for role %q", roleName)
		}

		inlinePolicyNames = append(inlinePolicyNames, res.PolicyNames...)
		if !res.IsTruncated {
			break
		}
		marker = res.Marker
	}

	for _, policyName := range inlinePolicyNames {
		_, err := client.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{
			PolicyName: aws.String(policyName),
			RoleName:   aws.String(roleName),
		})
		if err != nil {
			return errors.Annotatef(err, "delete inline policy %q", policyName)
		}
	}

	_, err := client.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	})

	return errors.Trace(err)
}

// ensureControllerInstanceProfile ensures that a controller Instance Profile
// has been created for the supplied controller name in the specified AWS cloud.
func ensureControllerInstanceProfile(
	ctx context.Context,
	client IAMClient,
	controllerName,
	controllerUUID string,
) (*iamtypes.InstanceProfile, []func(), error) {
	role, cleanups, err := ensureControllerInstanceRole(ctx, client, controllerName, controllerUUID)
	if err != nil {
		return nil, cleanups, err
	}

	profileName := fmt.Sprintf("juju-controller-%s", controllerName)
	res, err := client.CreateInstanceProfile(ctx, &iam.CreateInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
		Tags: []iamtypes.Tag{
			{
				Key:   aws.String(tags.JujuController),
				Value: aws.String(controllerUUID),
			},
		},
		Path: aws.String(controllerPath(controllerUUID)),
	})
	if err != nil {
		var alreadyExistsErr *iamtypes.EntityAlreadyExistsException
		if stderrors.As(err, &alreadyExistsErr) {
			// Instance Profile already exists so we don't need todo anything. Let just find it
			ip, err := findInstanceProfileFromName(ctx, client, profileName)
			return ip, cleanups, err
		}
		// Some other error that we can't recover from.
		return nil, cleanups, errors.Annotate(err, "creating controller instance profile")
	}

	cleanups = append([]func(){func() {
		_, err := client.DeleteInstanceProfile(ctx, &iam.DeleteInstanceProfileInput{
			InstanceProfileName: res.InstanceProfile.InstanceProfileName,
		})
		if err != nil {
			logger.Errorf(ctx, "cleanup delete instance profile %q: %v",
				*res.InstanceProfile.InstanceProfileName,
				err)
		}
	}}, cleanups...)

	_, err = client.AddRoleToInstanceProfile(ctx, &iam.AddRoleToInstanceProfileInput{
		InstanceProfileName: res.InstanceProfile.InstanceProfileName,
		RoleName:            role.RoleName,
	})

	if err != nil {
		return nil, cleanups, errors.Annotatef(
			err,
			"attaching role %s to instance profile %s",
			*role.RoleName,
			*res.InstanceProfile.InstanceProfileName,
		)
	}

	cleanups = append([]func(){func() {
		_, err := client.RemoveRoleFromInstanceProfile(ctx, &iam.RemoveRoleFromInstanceProfileInput{
			InstanceProfileName: res.InstanceProfile.InstanceProfileName,
			RoleName:            role.RoleName,
		})
		if err != nil {
			logger.Errorf(ctx, "cleanup remove role %q from instance profile %q: %v",
				*role.RoleName,
				*res.InstanceProfile.InstanceProfileName,
				err)
		}
	}}, cleanups...)

	return res.InstanceProfile, cleanups, nil
}

func ensureControllerInstanceRole(
	ctx context.Context,
	client IAMClient,
	controllerName,
	controllerUUID string,
) (*iamtypes.Role, []func(), error) {
	roleName := fmt.Sprintf("juju-controller-%s", controllerName)
	cleanups := []func(){}
	res, err := client.CreateRole(ctx, &iam.CreateRoleInput{
		AssumeRolePolicyDocument: aws.String(controllerRoleAssumePolicy),
		RoleName:                 aws.String(roleName),
		Description:              aws.String(fmt.Sprintf("juju role for controller %s", controllerName)),
		Path:                     aws.String(controllerPath(controllerUUID)),
		Tags: []iamtypes.Tag{
			{
				Key:   aws.String(tags.JujuController),
				Value: aws.String(controllerUUID),
			},
		},
	})

	if err != nil {
		var alreadyExistsErr *iamtypes.EntityAlreadyExistsException
		if stderrors.As(err, &alreadyExistsErr) {
			// Role already exists so we don't need todo anything. Let just find it
			r, err := findRoleFromName(ctx, client, roleName)
			return r, cleanups, err
		}
		// Some other error that we can't recover from.
		return nil, cleanups, errors.Annotate(err, "creating controller instance role")
	}

	cleanups = append(cleanups, func() {
		_, err := client.DeleteRole(ctx, &iam.DeleteRoleInput{
			RoleName: res.Role.RoleName,
		})
		if err != nil {
			logger.Errorf(ctx, "cleanup delete role %q: %v",
				*res.Role.RoleName,
				err)
		}
	})

	_, err = client.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		PolicyDocument: aws.String(controllerRolePolicy),
		PolicyName:     aws.String(roleName),
		RoleName:       res.Role.RoleName,
	})

	if err != nil {
		return nil, cleanups, errors.Annotatef(err, "attaching role %s policy", *res.Role.RoleName)
	}

	cleanups = append([]func(){func() {
		_, err := client.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{
			PolicyName: aws.String(roleName),
			RoleName:   res.Role.RoleName,
		})
		if err != nil {
			logger.Errorf(ctx, "cleanup delete role %q policy %q: %v",
				*res.Role.RoleName,
				roleName,
				err)
		}
	}}, cleanups...)

	return res.Role, cleanups, nil
}

// findInstanceProfileForName is responsible for finding the concrete instance
// profile for a supplied name. This is used to subsequently fetch the ARN of
// the InstanceProfile.
func findInstanceProfileFromName(
	ctx context.Context,
	client IAMClient,
	name string,
) (*iamtypes.InstanceProfile, error) {
	res, err := client.GetInstanceProfile(ctx, &iam.GetInstanceProfileInput{
		InstanceProfileName: &name,
	})

	if err != nil {
		if isAWSHTTPErrorCode(err, http.StatusNotFound) {
			return nil, errors.NotFoundf("instance profile %q not found", name)
		}
		return nil, errors.Annotatef(err, "finding instance profile for name %s", name)
	}

	return res.InstanceProfile, nil
}

func listInstanceProfilesForController(
	ctx context.Context,
	client IAMClient,
	controllerUUID string,
) ([]iamtypes.InstanceProfile, error) {
	var (
		marker *string
		rval   = []iamtypes.InstanceProfile{}
	)
	for {
		res, err := client.ListInstanceProfiles(ctx, &iam.ListInstanceProfilesInput{
			Marker:     marker,
			PathPrefix: aws.String(controllerPath(controllerUUID)),
		})

		if err != nil {
			if isAWSHTTPErrorCode(err, http.StatusForbidden) {
				return rval, errors.Unauthorizedf("listing instance profiles for controllerUUID %s", controllerUUID)
			}
			return rval, errors.Annotatef(
				err,
				"listing roles with controller prefix %q",
				controllerPath(controllerUUID))
		}

		rval = append(rval, res.InstanceProfiles...)
		if !res.IsTruncated {
			break
		}
		marker = res.Marker
	}
	return rval, nil
}

func listRolesForController(
	ctx context.Context,
	client IAMClient,
	controllerUUID string,
) ([]iamtypes.Role, error) {
	var (
		marker *string
		rval   = []iamtypes.Role{}
	)
	for {
		res, err := client.ListRoles(ctx, &iam.ListRolesInput{
			Marker:     marker,
			PathPrefix: aws.String(controllerPath(controllerUUID)),
		})

		if err != nil {
			if isAWSHTTPErrorCode(err, http.StatusForbidden) {
				return rval, errors.Unauthorizedf(
					"listing roles for controllerUUID %s",
					controllerUUID)
			}
			return rval, errors.Annotatef(
				err,
				"listing roles with controller prefix %q",
				controllerPath(controllerUUID))
		}

		rval = append(rval, res.Roles...)
		if !res.IsTruncated {
			break
		}
		marker = res.Marker
	}
	return rval, nil
}

func isAWSHTTPErrorCode(err error, statusCode int) bool {
	var opHTTPErr *awshttp.ResponseError
	if stderrors.As(err, &opHTTPErr) && opHTTPErr.HTTPStatusCode() == statusCode {
		return true
	}
	return false
}

func findRoleFromName(
	ctx context.Context,
	client IAMClient,
	name string,
) (*iamtypes.Role, error) {
	res, err := client.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(name),
	})

	if err != nil {
		if isAWSHTTPErrorCode(err, http.StatusNotFound) {
			return nil, errors.NotFoundf("role %q not found", name)
		}
		return nil, errors.Annotatef(err, "finding role for name %s", name)
	}

	return res.Role, nil
}

// setInstanceProfileWithWait sets the instnace profile for a given instance
// blocking until the instance is in a running state where the profile can be
// applied. This function also waits for the instance profile to be associated
// with the instance.
func setInstanceProfileWithWait(
	ctx context.Context,
	client instanceProfileClient,
	profile *iamtypes.InstanceProfile,
	inst instances.Instance,
	instLister environs.InstanceLister,
) error {
	var association *ec2.AssociateIamInstanceProfileOutput

	err := retry.Call(retry.CallArgs{
		Attempts: setProfileMaxAttempt,
		Delay:    setProfileDelay,
		Func: func() (err error) {
			association, err = setInstanceProfile(ctx, client, profile, inst, instLister)
			return err
		},
		IsFatalError: func(err error) bool {
			return !errors.Is(err, errors.NotProvisioned)
		},
		BackoffFunc: retry.DoubleDelay,
		Clock:       clock.WallClock,
		Stop:        ctx.Done(),
	})

	if err != nil {
		return errors.Annotatef(
			err,
			"setting instance profile %s for instance %s",
			*profile.InstanceProfileName,
			inst.Id(),
		)
	}

	// We need to wait here till the instance profile is associated to the
	// instance.
	return retry.Call(retry.CallArgs{
		Attempts: setProfileAssociationMaxAttempt,
		Delay:    setProfileAssociationDelay,
		Func: func() error {
			return IsInstanceProfileAssociated(
				ctx,
				client,
				*association.IamInstanceProfileAssociation.AssociationId,
				*association.IamInstanceProfileAssociation.InstanceId,
			)
		},
		IsFatalError: func(err error) bool {
			return !errors.Is(err, errors.NotProvisioned)
		},
		BackoffFunc: retry.DoubleDelay,
		Clock:       clock.WallClock,
		Stop:        ctx.Done(),
	})
}

func IsInstanceProfileAssociated(
	ctx context.Context,
	client instanceProfileClient,
	associationId,
	instanceId string,
) error {
	rval, err := client.DescribeIamInstanceProfileAssociations(
		ctx,
		&ec2.DescribeIamInstanceProfileAssociationsInput{
			AssociationIds: []string{
				associationId,
			},
			Filters: []ec2types.Filter{
				{
					Name: aws.String("instance-id"),
					Values: []string{
						instanceId,
					},
				},
			},
		},
	)

	if err != nil {
		return errors.Annotatef(
			err,
			"describing Instance Profile association %s",
			associationId,
		)
	}

	// We have only asked for one association from aws so getting back
	// more then one result doesn't make sense here so lets error. This
	// condition should never be hit.
	if len(rval.IamInstanceProfileAssociations) != 1 {
		return errors.Errorf("expected 1 IAM Instance Profile association, got %d", len(rval.IamInstanceProfileAssociations))
	}

	switch rval.IamInstanceProfileAssociations[0].State {
	case ec2types.IamInstanceProfileAssociationStateAssociated:
		return nil
	case ec2types.IamInstanceProfileAssociationStateAssociating:
		return errors.NotProvisionedf("IAM Instance Profile association %s", associationId)
	// This should only ever be hit if the association is being
	// Disassociated. This should never happen.
	default:
		return errors.NotSupportedf(" IAM Instance Profile association %s state %s",
			associationId,
			rval.IamInstanceProfileAssociations[0].State,
		)
	}
}

// setInstanceProfile sets the instance profile for a given instance. This
// function first checks to see that the supplied instance is in a running
// state first otherwise a Juju NotProvisioned error returned. Use
// setInstanceProfileWithWait to block on the instance status being running.
func setInstanceProfile(
	ctx context.Context,
	client instanceProfileClient,
	profile *iamtypes.InstanceProfile,
	inst instances.Instance,
	instLister environs.InstanceLister,
) (*ec2.AssociateIamInstanceProfileOutput, error) {
	rInst, err := instLister.Instances(ctx, []instance.Id{inst.Id()})
	if err != nil {
		return nil, errors.Annotatef(err, "listing instance with id %s", inst.Id())
	}
	if len(rInst) != 1 {
		return nil, errors.Errorf("expected 1 instance for id %s got %d", inst.Id(), len(rInst))
	}

	if rInst[0].Status(ctx).Status != status.Running {
		return nil, errors.NotProvisionedf("instance %s is not running", inst.Id())
	}

	instanceProfileInput := ec2.AssociateIamInstanceProfileInput{
		IamInstanceProfile: &ec2types.IamInstanceProfileSpecification{
			Arn:  profile.Arn,
			Name: profile.InstanceProfileName,
		},
		InstanceId: aws.String(string(inst.Id())),
	}

	rval, err := client.AssociateIamInstanceProfile(ctx, &instanceProfileInput)
	if err != nil {
		return nil, errors.Annotatef(
			err,
			"attaching instance profile %s to instance %s",
			*profile.InstanceProfileName,
			inst.Id())
	}

	return rval, nil
}
