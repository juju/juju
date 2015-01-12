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
}

// ClaimLeadershipBulkResults is the collection of results from a bulk
// leadership claim.
type ClaimLeadershipBulkResults struct {

	// Results is the collection of results from the claim.
	Results []ClaimLeadershipResults
}

// ClaimLeadershipResults are the results from making a leadership
// claim.
type ClaimLeadershipResults struct {

	// ServiceTag is the service for which you want to make a
	// leadership claim.
	ServiceTag string

	// ClaimDurationInSec is the number of seconds a claim will be
	// held.
	ClaimDurationInSec float64

	// Error is filled in if there was an error fulfilling the claim.
	Error *Error
}

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

// LeadershipSettings is a collection of settings only the leader of a
// service may write to.
//type LeadershipSettings map[string]interface{}
// GetLeadershipSettingsBulkParams is a collection of parameters for
// making a bulk request for leadership settings.
type GetLeadershipSettingsBulkParams struct {
	Params []GetLeadershipSettingsParams
}

// GetLeadershipSettingsParams are the parameters needed to request
// leadership settings for a service.
type GetLeadershipSettingsParams struct {
	ServiceTag string
}

// GetLeadershipSettingsBulkResults is the collection of results from
// a bulk request for leadership settings.
type GetLeadershipSettingsBulkResults struct {
	Results []GetLeadershipSettingsResult
}

// GetLeadershipSettingsResult is the results from requesting
// leadership settings.
type GetLeadershipSettingsResult struct {
	Settings map[string]interface{}
	Error    *Error
}
type LeadershipWatchSettingsBulkParams struct {
	Params []LeadershipWatchSettingsParam
}

type LeadershipWatchSettingsParam struct {
	ServiceTag string
}

type MergeLeadershipSettingsBulkParams struct {
	Params []MergeLeadershipSettingsParam
}

type MergeLeadershipSettingsParam struct {
	ServiceTag string
	Settings   map[string]interface{}
}
