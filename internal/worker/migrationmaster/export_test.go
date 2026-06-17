// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

// Expose the envelope conversion helpers so the external worker tests can
// build expected envelopes. The conversions themselves are unit tested in
// envelope_test.go.
var (
	EnvelopeFromControllerModelInfo = envelopeFromControllerModelInfo
	CharmURLsFromLocators           = charmURLsFromLocators
	ToolsForEnvelope                = toolsForEnvelope
	ResourcesForEnvelope            = resourcesForEnvelope
)
