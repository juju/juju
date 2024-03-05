// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// CreateTags implements ec2.Client.
func (srv *Server) CreateTags(ctx context.Context, in *ec2.CreateTagsInput, opts ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error) {
	srv.tagsMutatingCalls.next()

	if err, ok := srv.apiCallErrors["CreateTags"]; ok {
		return nil, err
	}
	// Each resource can have a maximum of 10 tags.
	const tagLimit = 10
	for _, resourceId := range in.Resources {
		resourceTags, err := srv.tags(resourceId)
		if err != nil {
			return nil, err
		}
		for _, tag := range in.Tags {
			var found bool
			for i := range *resourceTags {
				if aws.ToString((*resourceTags)[i].Key) != aws.ToString(tag.Key) {
					continue
				}
				(*resourceTags)[i].Value = tag.Value
				found = true
				break
			}
			if found {
				continue
			}
			if len(*resourceTags) == tagLimit {
				return nil, apiError("TagLimitExceeded", "The maximum number of Tags for a resource has been reached.")
			}
			*resourceTags = append(*resourceTags, tag)
		}
	}
	return &ec2.CreateTagsOutput{}, nil
}

func (srv *Server) tags(id string) (*[]types.Tag, error) {
	parts := strings.SplitN(id, "-", 2)
	if len(parts) == 0 {
		return nil, apiError("InvalidID", "The ID '%s' is not valid", id)
	}
	switch parts[0] {
	case "i":
		if inst, ok := srv.instances[id]; ok {
			return &inst.tags, nil
		}
	case "sg":
		if group, ok := srv.groups[id]; ok {
			return &group.tags, nil
		}
	case "vol":
		if vol, ok := srv.volumes[id]; ok {
			return &vol.Tags, nil
		}
		// TODO(axw) more resources as necessary.
	}
	return nil, apiError("InvalidID", "The ID '%s' is not valid", id)
}

func matchTag(tags []types.Tag, key, value string) bool {
	for _, tag := range tags {
		if aws.ToString(tag.Key) == key {
			return aws.ToString(tag.Value) == value
		}
	}
	return false
}

func tagSpecForType(
	resourceType types.ResourceType,
	specs []types.TagSpecification,
) (rval types.TagSpecification) {
	rval = types.TagSpecification{
		ResourceType: resourceType,
	}
	if specs == nil {
		return
	}

	for _, spec := range specs {
		if spec.ResourceType == resourceType {
			rval = spec
			return
		}
	}

	return rval
}
