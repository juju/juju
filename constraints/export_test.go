// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

var WithFallbacks = withFallbacks

func Without(cons Value, attrTags ...string) (Value, error) {
	return cons.without(attrTags...)
}

func HasAny(cons Value, attrTags ...string) []string {
	return cons.hasAny(attrTags...)
}

func AttributesWithValues(cons Value) map[string]interface{} {
	return cons.attributesWithValues()
}
