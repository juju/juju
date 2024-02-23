// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// SetAccountAttributes sets the given account attributes on the
// server. When the "default-vpc" attribute is specified, its value
// must match an existing VPC in the test server, otherwise it's an
// error. In addition, only the first value for "default-vpc", the
// rest (if any) are ignored.
func (srv *Server) SetAccountAttributes(attrs map[string][]types.AccountAttributeValue) error {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	for attrName, values := range attrs {
		srv.attributes[attrName] = values
		if attrName == "default-vpc" {
			if len(values) == 0 {
				return fmt.Errorf("no value(s) for attribute default-vpc")
			}
			defaultVPCId := aws.ToString(values[0].AttributeValue) // ignore the rest.
			if _, found := srv.vpcs[defaultVPCId]; !found {
				return fmt.Errorf("VPC %q not found", defaultVPCId)
			}
		}
	}
	return nil
}

// DescribeAccountAttributes implements ec2.Client.
func (srv *Server) DescribeAccountAttributes(ctx context.Context, in *ec2.DescribeAccountAttributesInput, opts ...func(*ec2.Options)) (*ec2.DescribeAccountAttributesOutput, error) {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	resp := &ec2.DescribeAccountAttributesOutput{}
	if in == nil {
		for name, vals := range srv.attributes {
			resp.AccountAttributes = append(resp.AccountAttributes, types.AccountAttribute{
				AttributeName:   aws.String(name),
				AttributeValues: vals,
			})
		}
		return resp, nil
	}
	for _, attrName := range in.AttributeNames {
		vals, ok := srv.attributes[string(attrName)]
		if !ok {
			return nil, apiError("InvalidParameterValue", "describe attrs: not found %q", attrName)
		}
		resp.AccountAttributes = append(resp.AccountAttributes, types.AccountAttribute{
			AttributeName:   aws.String(string(attrName)),
			AttributeValues: vals,
		})
	}
	return resp, nil
}
