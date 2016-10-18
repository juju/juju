// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth

import "strings"

// TokenResource returns a resource value suitable for auth tokens, based on
// an endpoint URI.
func TokenResource(uri string) string {
	resource := uri
	if !strings.HasSuffix(resource, "/") {
		resource += "/"
	}
	return resource
}
