// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import "github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/to"

func toTagsPtr(tags map[string]string) *map[string]*string {
	stringPtrMap := to.StringMapPtr(tags)
	return &stringPtrMap
}

func toTags(tags *map[string]*string) map[string]string {
	if tags == nil {
		return nil
	}
	return to.StringMap(*tags)
}

func toStringSlicePtr(s ...string) *[]string {
	return &s
}
