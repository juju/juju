// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The provider package holds constants identifying known provider types.
// They have hitherto only been used for nefarious purposes; no new code
// should use them, and when old code is updated to no longer use them
// they must be deleted.
package provider

const (
	Joyent = "joyent"
	Local  = "local"
	MAAS   = "maas"
	EC2    = "ec2"
)

// IsManual returns true iff the specified provider
// type refers to the manual provider.
func IsManual(provider string) bool {
	switch provider {
	case "null", "manual":
		return true
	default:
		return false
	}
}
