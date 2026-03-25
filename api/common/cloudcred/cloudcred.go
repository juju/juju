// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:generate go run ../../../generate/cloudcred/main.go -o attr.go -p cloudcred

package cloudcred

import "fmt"

// IsVisibleAttribute returns whether a cloud-credential attribute is known
// not to be hidden and therefore does not need to be redacted.
func IsVisibleAttribute(provider, authtype, attribute string) bool {
	return attr[fmt.Sprintf("%s,%s,%s", provider, authtype, attribute)]
}
