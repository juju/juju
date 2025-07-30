// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

type ResourceTagInfo struct {
	BaseResourceTags string
	ModelUUID        string
	ControllerUUID   string
	ApplicationName  string
}
