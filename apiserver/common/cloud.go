// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
)

func CloudToParams(cloud jujucloud.Cloud) params.Cloud {
	authTypes := make([]string, len(cloud.AuthTypes))
	for i, authType := range cloud.AuthTypes {
		authTypes[i] = string(authType)
	}
	regions := make([]params.CloudRegion, len(cloud.Regions))
	for i, region := range cloud.Regions {
		regions[i] = params.CloudRegion{
			Name:             region.Name,
			Endpoint:         region.Endpoint,
			IdentityEndpoint: region.IdentityEndpoint,
			StorageEndpoint:  region.StorageEndpoint,
		}
	}
	return params.Cloud{
		Type:             cloud.Type,
		AuthTypes:        authTypes,
		Endpoint:         cloud.Endpoint,
		IdentityEndpoint: cloud.IdentityEndpoint,
		StorageEndpoint:  cloud.StorageEndpoint,
		Regions:          regions,
	}
}

func CloudFromParams(cloudName string, p params.Cloud) jujucloud.Cloud {
	authTypes := make([]jujucloud.AuthType, len(p.AuthTypes))
	for i, authType := range p.AuthTypes {
		authTypes[i] = jujucloud.AuthType(authType)
	}
	regions := make([]jujucloud.Region, len(p.Regions))
	for i, region := range p.Regions {
		regions[i] = jujucloud.Region{
			Name:             region.Name,
			Endpoint:         region.Endpoint,
			IdentityEndpoint: region.IdentityEndpoint,
			StorageEndpoint:  region.StorageEndpoint,
		}
	}
	return jujucloud.Cloud{
		Name:             cloudName,
		Type:             p.Type,
		AuthTypes:        authTypes,
		Endpoint:         p.Endpoint,
		IdentityEndpoint: p.IdentityEndpoint,
		StorageEndpoint:  p.StorageEndpoint,
		Regions:          regions,
	}
}
