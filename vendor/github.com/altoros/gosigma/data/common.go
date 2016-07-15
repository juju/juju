// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package data

import "fmt"

// Meta describes properties of dataset
type Meta struct {
	Limit      int `json:"limit"`
	Offset     int `json:"offset"`
	TotalCount int `json:"total_count"`
}

// Resource describes properties of linked resource
type Resource struct {
	URI  string `json:"resource_uri,omitempty"`
	UUID string `json:"uuid,omitempty"`
}

// MakeResource returns Resource structure from given type and UUID
func MakeResource(t, uuid string) *Resource {
	return &Resource{
		URI:  fmt.Sprintf("/api/2.0/%s/%s/", t, uuid),
		UUID: uuid,
	}
}

// MakeDriveResource returns drive Resource structure for given UUID
func MakeDriveResource(uuid string) *Resource {
	return MakeResource("drives", uuid)
}

// MakeJobResource returns job Resource structure for given UUID
func MakeJobResource(uuid string) *Resource {
	return MakeResource("jobs", uuid)
}

// MakeIPResource returns IP Resource structure for given IP address
func MakeIPResource(ip string) *Resource {
	return MakeResource("ips", ip)
}

// MakeLibDriveResource returns library drive Resource structure for given UUID
func MakeLibDriveResource(uuid string) *Resource {
	return MakeResource("libdrives", uuid)
}

// MakeServerResource returns server Resource structure for given UUID
func MakeServerResource(uuid string) *Resource {
	return MakeResource("servers", uuid)
}

// MakeUserResource returns user Resource structure for given UUID
func MakeUserResource(uuid string) *Resource {
	return MakeResource("user", uuid)
}

// MakeVLanResource returns VLan Resource structure for given UUID
func MakeVLanResource(uuid string) *Resource {
	return MakeResource("vlans", uuid)
}
