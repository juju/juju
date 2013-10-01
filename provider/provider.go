// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The provider package holds constants identifying known provider types.
// They have hitherto only been used for nefarious purposes; no new code
// should use them, and when old code is updated to no longer use them
// they must be deleted.
package provider

const (
	Local = "local"
	Null  = "null"
)
