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

func (i *IAMServer) CreateRole(
	ctx context.Context,
	input *iam.CreateRoleInput,
	opts ...func(*iam.Options),
) (*iam.CreateRoleOutput, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if role, exists := i.roles[*input.RoleName]; exists {
		return &iam.CreateRoleOutput{
				Role: role,
			}, &types.EntityAlreadyExistsException{
				Message: aws.String(fmt.Sprintf("role %s", *input.RoleName)),
			}
	}

	createDate := time.Now()
	i.roles[*input.RoleName] = &types.Role{
		Arn:                      input.RoleName,
		CreateDate:               &createDate,
		RoleName:                 input.RoleName,
		AssumeRolePolicyDocument: input.AssumeRolePolicyDocument,
		Description:              input.Description,
		MaxSessionDuration:       input.MaxSessionDuration,
		Path:                     input.Path,
		Tags:                     input.Tags,
	}

	return &iam.CreateRoleOutput{
		Role: i.roles[*input.RoleName],
	}, nil
}

func (i *IAMServer) DeleteRole(
	ctx context.Context,
	input *iam.DeleteRoleInput,
	opts ...func(*iam.Options),
) (*iam.DeleteRoleOutput, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if _, exists := i.roles[*input.RoleName]; !exists {
		return nil, apiError("InvalidRole.NotFound", "role not found")
	}

	delete(i.roleInlinePolicy, *input.RoleName)
	delete(i.roles, *input.RoleName)
	return &iam.DeleteRoleOutput{}, nil
}

func (i *IAMServer) DeleteRolePolicy(
	ctx context.Context,
	input *iam.DeleteRolePolicyInput,
	opts ...func(*iam.Options),
) (*iam.DeleteRolePolicyOutput, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	inlinePolicy, exists := i.roleInlinePolicy[*input.RoleName]
	if !exists {
		return nil, apiError("InvalidRolePolicy.NotFound", "role has no policy")
	}

	if *inlinePolicy.PolicyName != *input.PolicyName {
		return nil, apiError("InvalidRolePolicy.NotFound", "role has no policy")
	}

	delete(i.roleInlinePolicy, *input.RoleName)
	return &iam.DeleteRolePolicyOutput{}, nil
}

func (i *IAMServer) GetRole(
	ctx context.Context,
	input *iam.GetRoleInput,
	opts ...func(*iam.Options),
) (*iam.GetRoleOutput, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	role, exists := i.roles[*input.RoleName]
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
	return &iam.GetRoleOutput{
		Role: role,
	}, nil
}

func (i *IAMServer) PutRolePolicy(
	ctx context.Context,
	input *iam.PutRolePolicyInput,
	opts ...func(*iam.Options),
) (*iam.PutRolePolicyOutput, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	role, exists := i.roles[*input.RoleName]
	if !exists {
		return nil, apiError("InvalidRole.NotFound", "role not found")
	}

	inlinePolicy := &InlinePolicy{
		PolicyDocument: input.PolicyDocument,
		PolicyName:     input.PolicyName,
	}

	i.roleInlinePolicy[*role.RoleName] = inlinePolicy
	return &iam.PutRolePolicyOutput{}, nil
}
