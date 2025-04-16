// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/rpc/params"
)

// CloudSpecToParams constructs a params cloud spec from a cloud spec.
func CloudSpecToParams(spec environscloudspec.CloudSpec) *params.CloudSpec {
	var paramsCloudCredential *params.CloudCredential
	if spec.Credential != nil && spec.Credential.AuthType() != "" {
		paramsCloudCredential = &params.CloudCredential{
			AuthType:   string(spec.Credential.AuthType()),
			Attributes: spec.Credential.Attributes(),
		}
	}
	return &params.CloudSpec{
		Type:              spec.Type,
		Name:              spec.Name,
		Region:            spec.Region,
		Endpoint:          spec.Endpoint,
		IdentityEndpoint:  spec.IdentityEndpoint,
		StorageEndpoint:   spec.StorageEndpoint,
		Credential:        paramsCloudCredential,
		CACertificates:    spec.CACertificates,
		SkipTLSVerify:     spec.SkipTLSVerify,
		IsControllerCloud: spec.IsControllerCloud,
	}
}
