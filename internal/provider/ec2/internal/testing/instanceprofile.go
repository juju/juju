// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func (i *IAMServer) AddRoleToInstanceProfile(
	ctx context.Context,
	input *iam.AddRoleToInstanceProfileInput,
	opts ...func(*iam.Options),
) (*iam.AddRoleToInstanceProfileOutput, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	role, exists := i.roles[*input.RoleName]
	if !exists {
		return nil, apiError("InvalidRole.NotFound", "role not found")
	}

	instanceProfile, exists := i.instanceProfiles[*input.InstanceProfileName]
	if !exists {
		return nil, apiError("InvalidInstanceProfile.NotFound", "instance profile not found")
	}

	if len(instanceProfile.Roles) != 0 {
		return nil, apiError("InstanceProfile", "already has role attached")
	}

	instanceProfile.Roles = []types.Role{*role}
	return &iam.AddRoleToInstanceProfileOutput{}, nil
}

func (i *IAMServer) CreateInstanceProfile(
	ctx context.Context,
	input *iam.CreateInstanceProfileInput,
	opts ...func(*iam.Options),
) (*iam.CreateInstanceProfileOutput, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if ip, exists := i.instanceProfiles[*input.InstanceProfileName]; exists {
		return &iam.CreateInstanceProfileOutput{
				InstanceProfile: ip,
			}, &types.EntityAlreadyExistsException{
				Message: aws.String(fmt.Sprintf("instance profile %s", *input.InstanceProfileName)),
			}
	}

	createDate := time.Now()
	i.instanceProfiles[*input.InstanceProfileName] = &types.InstanceProfile{
		Arn:                 input.InstanceProfileName,
		CreateDate:          &createDate,
		InstanceProfileName: input.InstanceProfileName,
		Path:                input.Path,
		Tags:                input.Tags,
	}
	return &iam.CreateInstanceProfileOutput{
		InstanceProfile: i.instanceProfiles[*input.InstanceProfileName],
	}, nil
}

func (i *IAMServer) DeleteInstanceProfile(
	ctx context.Context,
	input *iam.DeleteInstanceProfileInput,
	opts ...func(*iam.Options),
) (*iam.DeleteInstanceProfileOutput, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if _, exists := i.instanceProfiles[*input.InstanceProfileName]; !exists {
		return nil, apiError("InvalidInstanceProfile.NotFound", "instance profile not found")
	}

	delete(i.instanceProfiles, *input.InstanceProfileName)
	return &iam.DeleteInstanceProfileOutput{}, nil
}

func (i *IAMServer) GetInstanceProfile(
	ctx context.Context,
	input *iam.GetInstanceProfileInput,
	opts ...func(*iam.Options),
) (*iam.GetInstanceProfileOutput, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	ip, exists := i.instanceProfiles[*input.InstanceProfileName]
	if !exists {
		return nil, &awshttp.ResponseError{
			ResponseError: &smithyhttp.ResponseError{
				Response: &smithyhttp.Response{
					&http.Response{
						StatusCode: http.StatusNotFound,
					},
				},
			},
		}
	}

	return &iam.GetInstanceProfileOutput{
		InstanceProfile: ip,
	}, nil
}

func (i *IAMServer) ListInstanceProfiles(
	ctx context.Context,
	input *iam.ListInstanceProfilesInput,
	opts ...func(*iam.Options),
) (*iam.ListInstanceProfilesOutput, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	rval := &iam.ListInstanceProfilesOutput{
		InstanceProfiles: []types.InstanceProfile{},
		IsTruncated:      false,
	}

	if i.producePermissionError {
		return rval, &awshttp.ResponseError{
			ResponseError: &smithyhttp.ResponseError{
				Response: &smithyhttp.Response{
					&http.Response{
						StatusCode: http.StatusForbidden,
					},
				},
			},
		}
	}

	for _, v := range i.instanceProfiles {
		rval.InstanceProfiles = append(rval.InstanceProfiles, *v)
	}

	return rval, nil
}

func (i *IAMServer) RemoveRoleFromInstanceProfile(
	ctx context.Context,
	input *iam.RemoveRoleFromInstanceProfileInput,
	opts ...func(*iam.Options),
) (*iam.RemoveRoleFromInstanceProfileOutput, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	ip, exists := i.instanceProfiles[*input.InstanceProfileName]
	if !exists {
		return nil, apiError("InvalidInstanceProfile.NotFound", "instance profile not found")
	}

	if len(ip.Roles) > 1 {
		return nil, apiError("InvalidInstanceProfile.RoleCount", "Instance profile has more then 1 role")
	} else if len(ip.Roles) == 0 {
		return nil, apiError("InvalidInstanceProfile.NoRole", "Instance profile has no role attached")
	}

	if *ip.Roles[0].RoleName != *input.RoleName {
		return nil, apiError(
			"InvalidInstanceProfile.Role",
			"role %s is not attached to instance profile", *ip.Roles[1].RoleName)
	}

	ip.Roles = []types.Role{}
	return &iam.RemoveRoleFromInstanceProfileOutput{}, nil
}
