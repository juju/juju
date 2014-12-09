// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/names"
)

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
	ServiceTag names.ServiceTag

	// UnitTag is the unit which is making the leadership claim.
	UnitTag names.UnitTag
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
	ServiceTag names.ServiceTag

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
	ServiceTag names.ServiceTag

	// UnitTag is the unit which is making the leadership claim.
	UnitTag names.UnitTag
}

// ReleaseLeadershipBulkResults is a type which contains results from
// a bulk leadership call.
type ReleaseLeadershipBulkResults struct {

	// Errors represents errors which may have occurred for each
	// release. The indexes correspond to the parameters passed into
	// the call.
	Errors []*Error
}
