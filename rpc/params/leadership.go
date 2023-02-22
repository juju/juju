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

// PinApplicationsResults returns all applications for which pinning or
// unpinning was attempted, including any errors that occurred.
type PinApplicationsResults struct {
	// Results is collection with each application tag and pin/unpin result.
	Results []PinApplicationResult `json:"results"`
}

// PinApplicationResult represents the result of a single application
// leadership pin/unpin operation
type PinApplicationResult struct {
	// ApplicationName is the application for which a leadership pin/unpin
	// operation was attempted.
	ApplicationName string `json:"application-name"`
	// Error will contain a reference to an error resulting from pin/unpin
	// if one occurred.
	Error *Error `json:"error,omitempty"`
}

// PinnedLeadershipResult holds data representing the current applications for
// which leadership is pinned
type PinnedLeadershipResult struct {
	// Result has:
	// - Application name keys representing the application pinned.
	// - Tag slice values representing the entities requiring pinned
	//   behaviour for each application.
	Result map[string][]string `json:"result,omitempty"`

	// Error will contain a reference to an error resulting from
	// reading lease data, if one occurred.
	Error *Error `json:"error,omitempty"`
}
