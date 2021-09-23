// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"
	"net/http"
	time "time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type IAMSuite struct{}

type mockIAMClient struct {
	createInstanceProfileFn func(context.Context, *iam.CreateInstanceProfileInput, ...func(*iam.Options)) (*iam.CreateInstanceProfileOutput, error)
	getInstanceProfileFn    func(context.Context, *iam.GetInstanceProfileInput, ...func(*iam.Options)) (*iam.GetInstanceProfileOutput, error)
}

var _ = gc.Suite(&IAMSuite{})

func (m *mockIAMClient) CreateInstanceProfile(
	c context.Context,
	i *iam.CreateInstanceProfileInput,
	o ...func(*iam.Options),
) (*iam.CreateInstanceProfileOutput, error) {
	if m.createInstanceProfileFn == nil {
		return nil, errors.NewNotImplemented(nil, "mockIAMClient has no createInstanceProfileFn set")
	}
	return m.createInstanceProfileFn(c, i, o...)
}

func (m *mockIAMClient) GetInstanceProfile(
	c context.Context,
	i *iam.GetInstanceProfileInput,
	o ...func(*iam.Options),
) (*iam.GetInstanceProfileOutput, error) {
	if m.getInstanceProfileFn == nil {
		return nil, errors.NewNotImplemented(nil, "mockIAMClient has no getInstanceProfileFn set")
	}
	return m.getInstanceProfileFn(c, i, o...)
}

func (*IAMSuite) TestEnsureControllerInstanceProfileFromScratch(c *gc.C) {
	client := &mockIAMClient{
		createInstanceProfileFn: func(
			_ context.Context,
			i *iam.CreateInstanceProfileInput,
			_ ...func(*iam.Options)) (*iam.CreateInstanceProfileOutput, error) {

			c.Assert(*i.InstanceProfileName, gc.Equals, "juju-controller-test")
			c.Assert(i.Path, gc.IsNil)
			c.Assert(len(i.Tags), gc.Equals, 0)

			t := time.Now()
			return &iam.CreateInstanceProfileOutput{
				InstanceProfile: &types.InstanceProfile{
					Arn:                 aws.String("arn://12345"),
					CreateDate:          &t,
					InstanceProfileName: i.InstanceProfileName,
				},
			}, nil
		},
	}

	_, err := ensureControllerInstanceProfile(context.TODO(), client, "test")
	c.Assert(err, jc.ErrorIsNil)
}

func (*IAMSuite) TestEnsureControllerInstanceProfileAlreadyExists(c *gc.C) {
	getInstanceProfileCalled := false

	client := &mockIAMClient{
		createInstanceProfileFn: func(
			_ context.Context,
			i *iam.CreateInstanceProfileInput,
			_ ...func(*iam.Options)) (*iam.CreateInstanceProfileOutput, error) {

			c.Assert(*i.InstanceProfileName, gc.Equals, "juju-controller-test")
			c.Assert(i.Path, gc.IsNil)
			c.Assert(len(i.Tags), gc.Equals, 0)

			return nil, &types.EntityAlreadyExistsException{
				Message: aws.String("already exists"),
			}
		},
		getInstanceProfileFn: func(
			_ context.Context,
			i *iam.GetInstanceProfileInput,
			_ ...func(*iam.Options)) (*iam.GetInstanceProfileOutput, error) {
			getInstanceProfileCalled = true

			c.Assert(*i.InstanceProfileName, gc.Equals, "juju-controller-test")

			t := time.Now()
			return &iam.GetInstanceProfileOutput{
				InstanceProfile: &types.InstanceProfile{
					Arn:                 aws.String("arn://12345"),
					CreateDate:          &t,
					InstanceProfileName: i.InstanceProfileName,
				},
			}, nil
		},
	}

	instanceProfile, err := ensureControllerInstanceProfile(context.TODO(), client, "test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(getInstanceProfileCalled, jc.IsTrue)
	c.Assert(*instanceProfile.Arn, gc.Equals, "arn://12345")
	c.Assert(*instanceProfile.InstanceProfileName, gc.Equals, "juju-controller-test")
}

func (*IAMSuite) TestFindInstanceProfileExists(c *gc.C) {
	client := &mockIAMClient{
		getInstanceProfileFn: func(
			_ context.Context,
			i *iam.GetInstanceProfileInput,
			_ ...func(*iam.Options)) (*iam.GetInstanceProfileOutput, error) {

			c.Assert(*i.InstanceProfileName, gc.Equals, "test")
			t := time.Now()
			return &iam.GetInstanceProfileOutput{
				InstanceProfile: &types.InstanceProfile{
					Arn:                 aws.String("arn://12345"),
					CreateDate:          &t,
					InstanceProfileName: i.InstanceProfileName,
				},
			}, nil
		},
	}

	instanceProfile, err := findInstanceProfileFromName(context.TODO(), client, "test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*instanceProfile.Arn, gc.Equals, "arn://12345")
	c.Assert(*instanceProfile.InstanceProfileName, gc.Equals, "test")
}

func (*IAMSuite) TestFindInstanceProfileWithNotFoundError(c *gc.C) {
	client := &mockIAMClient{
		getInstanceProfileFn: func(
			_ context.Context,
			i *iam.GetInstanceProfileInput,
			_ ...func(*iam.Options)) (*iam.GetInstanceProfileOutput, error) {

			c.Assert(*i.InstanceProfileName, gc.Equals, "test")
			return nil, &awshttp.ResponseError{
				ResponseError: &smithyhttp.ResponseError{
					Response: &smithyhttp.Response{
						&http.Response{
							StatusCode: http.StatusNotFound,
						},
					},
				},
			}
		},
	}

	instanceProfile, err := findInstanceProfileFromName(context.TODO(), client, "test")
	c.Assert(instanceProfile, gc.IsNil)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func (*IAMSuite) TestFindInstanceProfileWithError(c *gc.C) {
	rErr := errors.New("test error")

	client := &mockIAMClient{
		getInstanceProfileFn: func(
			_ context.Context,
			i *iam.GetInstanceProfileInput,
			_ ...func(*iam.Options)) (*iam.GetInstanceProfileOutput, error) {

			c.Assert(*i.InstanceProfileName, gc.Equals, "test")
			return nil, rErr
		},
	}

	instanceProfile, err := findInstanceProfileFromName(context.TODO(), client, "test")
	c.Assert(instanceProfile, gc.IsNil)
	c.Assert(err.Error(), gc.Equals, "finding instance profile for name test: test error")
}
