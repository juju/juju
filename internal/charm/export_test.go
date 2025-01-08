// Copyright 2011-2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

// Export meaningful bits for tests only.

var (
	IfaceExpander = ifaceExpander

	ResourceSchema            = resourceSchema
	ExtraBindingsSchema       = extraBindingsSchema
	ValidateMetaExtraBindings = validateMetaExtraBindings
	ParseResourceMeta         = parseResourceMeta
)
