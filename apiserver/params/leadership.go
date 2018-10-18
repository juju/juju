// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// ClaimLeadershipBulkParams is a collection of parameters for making
// a bulk leadership claim.
type ClaimLeadershipBulkParams struct {

	// Params are the parameters for making a bulk leadership claim.
	Params []ClaimLeadershipParams `json:"params"`
}

// ClaimLeadershipParams are the parameters needed for making a
// leadership claim.
type ClaimLeadershipParams struct {

	// ApplicationTag is the application for which you want to make a
	// leadership claim.
	ApplicationTag string `json:"application-tag"`

	// UnitTag is the unit which is making the leadership claim.
	UnitTag string `json:"unit-tag"`

	// DurationSeconds is the number of seconds for which the lease is required.
	DurationSeconds float64 `json:"duration"`
}

// ClaimLeadershipBulkResults is the collection of results from a bulk
// leadership claim.
type ClaimLeadershipBulkResults ErrorResults

// GetLeadershipSettingsBulkResults is the collection of results from
// a bulk request for leadership settings.
type GetLeadershipSettingsBulkResults struct {
	Results []GetLeadershipSettingsResult `json:"results"`
}

// GetLeadershipSettingsResult is the results from requesting
// leadership settings.
type GetLeadershipSettingsResult struct {
	Settings Settings `json:"settings"`
	Error    *Error   `json:"error,omitempty"`
}

// MergeLeadershipSettingsBulkParams is a collection of parameters for
// making a bulk merge of leadership settings.
type MergeLeadershipSettingsBulkParams struct {

	// Params are the parameters for making a bulk leadership settings
	// merge.
	Params []MergeLeadershipSettingsParam `json:"params"`
}

// MergeLeadershipSettingsParam are the parameters needed for merging
// in leadership settings.
type MergeLeadershipSettingsParam struct {
	// ApplicationTag is the application for which you want to merge
	// leadership settings.
	ApplicationTag string `json:"application-tag,omitempty"`

	// UnitTag is the unit for which you want to merge
	// leadership settings.
	UnitTag string `json:"unit-tag,omitempty"`

	// Settings are the Leadership settings you wish to merge in.
	Settings Settings `json:"settings"`
}

// PinLeadershipBulkParams is a collection of parameters for making
// bulk requests to pin or unpin leadership claims.
type PinLeadershipBulkParams struct {

	// Params indicates the collection of applications for which a pin or unpin
	// request is being made.
	Params []PinLeadershipParams `json:"params"`
}

// PinLeadershipParams holds the parameters for pinning or unpinning a single
// application's leadership claim.
type PinLeadershipParams struct {
	// ApplicationTag is the application for which leadership is to
	// be pinned or unpinned.
	ApplicationTag string `json:"application-tag"`

	// EntityTag denotes who is responsible for particular pin being
	// operated on.
	EntityTag string `json:"entity-tag"`
}
