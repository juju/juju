// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"strings"

	"github.com/juju/juju/environs/config"
)

// CoerceConfigForStorage is responsible for taking a config object and
// distilling all of its attribute values down to strings for persistence. The
// best way to have complex types go through this function is for type to
// implement the stringer interface.
//
// TODO tlm: This function is really bad and should be removed with some work.
// Specifically the services layer should not be trying to understand the
// default in and outs of every config type and how to get it into shape for
// persistence. Instead, the config should be providing translation helpers to
// deal with this so that each attribute can be encapsulated on its own.
// This is a leftover copy from Mongo and should be dealt with at some stage.
func CoerceConfigForStorage(attrs map[string]any) (map[string]string, error) {
	coerced := make(map[string]string, len(attrs))

	for attrName, attrValue := range attrs {
		if attrName == config.ResourceTagsKey {
			// Resource Tags are specified by the user as a string but transformed
			// to a map when config is parsed. We want to store as a string.
			var tagsSlice []string
			if tags, ok := attrValue.(map[string]string); ok {
				for resKey, resValue := range tags {
					tagsSlice = append(tagsSlice, fmt.Sprintf("%v=%v", resKey, resValue))
				}
				attrValue = strings.Join(tagsSlice, " ")
			}
		}
		coerced[attrName] = fmt.Sprintf("%v", attrValue)
	}
	return coerced, nil
}
