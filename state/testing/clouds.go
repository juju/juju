// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import "github.com/juju/juju/cloud"

func TestClouds() map[string]cloud.Cloud {
	return map[string]cloud.Cloud{
		"dummy": {
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		"dummy-regions": {
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
			Regions: []cloud.Region{{
				Name: "region-1",
			}, {
				Name: "region-2",
			}},
		},
		"dummy-auth": {
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.UserPassAuthType},
			Regions: []cloud.Region{{
				Name: "region-1",
			}, {
				Name: "region-2",
			}},
		},
	}
}

func TestCredentials() map[string]cloud.CloudCredential {
	return map[string]cloud.CloudCredential{
		"dummy-auth": {
			DefaultCredential: "street",
			DefaultRegion:     "region-1",
			AuthCredentials: map[string]cloud.Credential{
				"street": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
					"username": "vulture",
					"password": "hunter2",
				}),
			},
		},
	}
}
