// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Until we have a better way, we'll store information for supported public clouds here
// so that the user does not have to configure their own tools-url.

package openstack

import (
	"strings"
)

var toolsURLs = map[string]string{
	// HP Cloud
	"https://region-a.geo-1.identity.hpcloudsvc.com:35357/v2.0/": "https://region-a.geo-1.objects.hpcloudsvc.com:443/v1/60502529753910/juju-dist/tools",
}

// GetCertifiedToolsURL returns the tools URL relevant to the cloud with the specified auth URL.
func GetCertifiedToolsURL(auth_url string) (string, bool) {
	if !strings.HasSuffix(auth_url, "/") {
		auth_url = auth_url + "/"
	}
	url, ok := toolsURLs[auth_url]
	return url, ok
}
