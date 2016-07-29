// Copyright 2012-2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"fmt"
	"net/http"
)

func getVLANsEndpoint(version string) string {
	return fmt.Sprintf("/api/%s/vlans/", version)
}

// TestVLAN is the MAAS API VLAN representation
type TestVLAN struct {
	Name   string `json:"name"`
	Fabric string `json:"fabric"`
	VID    uint   `json:"vid"`

	ResourceURI string `json:"resource_uri"`
	ID          uint   `json:"id"`
}

// PostedVLAN is the MAAS API posted VLAN representation
type PostedVLAN struct {
	Name string `json:"name"`
	VID  uint   `json:"vid"`
}

func vlansHandler(server *TestServer, w http.ResponseWriter, r *http.Request) {
	//TODO
}
