// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// ClaimLeadershipBulkParams is a collection of parameters for making
// a bulk leadership claim.
type ClaimLeadershipBulkParams struct {

	// Params are the parameters for making a bulk leadership claim.
	Params []ClaimLeadershipParams
}

// ClaimLeadershipParams are the parameters needed for making a
// leadership claim.
type ClaimLeadershipParams struct {

	// ServiceTag is the service for which you want to make a
	// leadership claim.
	ServiceTag string

	// UnitTag is the unit which is making the leadership claim.
	UnitTag string

	// DurationSeconds is the number of seconds for which the lease is required.
	DurationSeconds float64
}

// ClaimLeadershipBulkResults is the collection of results from a bulk
// leadership claim.
type ClaimLeadershipBulkResults ErrorResults

// ReleaseLeadershipBulkParams is a collection of parameters needed to
// make a bulk release leadership call.
type ReleaseLeadershipBulkParams struct {
	Params []ReleaseLeadershipParams
}

// ReleaseLeadershipParams are the parameters needed to release a
// leadership claim.
type ReleaseLeadershipParams struct {

	// ServiceTag is the service for which you want to make a
	// leadership claim.
	ServiceTag string

	// UnitTag is the unit which is making the leadership claim.
	UnitTag string
}

// ReleaseLeadershipBulkResults is a type which contains results from
// a bulk leadership call.
type ReleaseLeadershipBulkResults ErrorResults

// GetLeadershipSettingsBulkResults is the collection of results from
// a bulk request for leadership settings.
type GetLeadershipSettingsBulkResults struct {
	Results []GetLeadershipSettingsResult
}

// GetLeadershipSettingsResult is the results from requesting
// leadership settings.
type GetLeadershipSettingsResult struct {
	Settings Settings
	Error    *Error
}

// MergeLeadershipSettingsBulkParams is a collection of parameters for
// making a bulk merge of leadership settings.
type MergeLeadershipSettingsBulkParams struct {

	// Params are the parameters for making a bulk leadership settings
	// merge.
	Params []MergeLeadershipSettingsParam
}

// MergeLeadershipSettingsParam are the parameters needed for merging
// in leadership settings.
type MergeLeadershipSettingsParam struct {
	// ServiceTag is the service for which you want to merge
	// leadership settings.
	ServiceTag string

	// Settings are the Leadership settings you wish to merge in.
	Settings Settings
}
