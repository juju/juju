// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// TODO(ericsnow) Eliminate the dependence on apiserver if possible.

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

// ListSpecsArgs are the arguments for the ListSpecs endpoint.
type ListSpecsArgs params.Entities

// NewListSpecsArgs returns the arguments for the ListSpecs endpoint.
func NewListSpecsArgs(services ...string) (ListSpecsArgs, error) {
	var args ListSpecsArgs
	for _, service := range services {
		if !names.IsValidService(service) {
			return args, errors.Errorf("invalid service %q", service)
		}

		args.Entities = append(args.Entities, params.Entity{
			Tag: names.NewServiceTag(service).String(),
		})
	}
	return args, nil
}

// SpecsResults holds the specs that result from a bulk API call.
type SpecsResults struct {
	// Results is the list of resource results.
	Results []SpecsResult
}

// SpecsResult holds the specs that result from an API call
// for a single service.
type SpecsResult struct {
	params.Entity
	params.ErrorResult

	// Specs is the list of specs for the service.
	Specs []ResourceSpec
}

// NewSpecsResult produces a SpecsResult for the given service tag. The
// corresponding service ID is also returned. If any error results, it
// is stored in the Error field of the result.
func NewSpecsResult(tagStr string) (SpecsResult, string) {
	var result SpecsResult
	result.Tag = tagStr

	if !names.IsValidService(tagStr) {
		result.Error = &params.Error{
			Message: fmt.Sprintf("invalid service tag %q", tagStr),
			Code:    params.CodeBadRequest,
		}
		return result, ""
	}

	tag, err := names.ParseTag(tagStr)
	if err != nil {
		result.Error = &params.Error{
			Message: fmt.Sprintf("unexpectedly failed to parse service tag %q", tagStr),
			Code:    params.CodeBadRequest,
		}
		return result, ""
	}

	return result, tag.Id()
}

// SetResultError sets the error on the result.
func SetResultError(result *SpecsResult, err error) {
	result.Error = common.ServerError(err)
}

// ResourceSpec contains the definition for a resource.
type ResourceSpec struct {
	// Name identifies the resource.
	Name string `json:"name"`

	// Type is the name of the resource type.
	Type string `json:"type"`

	// Path is where the resource will be stored.
	Path string `json:"path"`

	// Comment contains user-facing info about the resource.
	Comment string `json:"comment,omitempty"`

	// Origin is where the resource will come from.
	Origin string `json:"origin"`

	// Revision is the desired revision, if applicable.
	Revision string `json:"revision"`
}
