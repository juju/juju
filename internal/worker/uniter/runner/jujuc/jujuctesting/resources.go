// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

// ContextResources is a test double for jujuc.ContextResources.
type ContextResources struct {
	contextBase
}

// DownloadResource implements jujuc.ContextResources.
func (c *ContextRelations) DownloadResource(resourceName string) (string, error) {
	c.stub.AddCall("DownloadResource", resourceName)
	return "/path/to/" + resourceName, nil
}
