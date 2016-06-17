// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"
)

// LogFwdLastSentID is the API data that identifies a log forwarding
// "last sent" value. The controller has a mapping from a set of IDs
// to a timestamp (for each ID). The timestamp corresponds to the last
// log record that the specific log forwarding machinery sent to the
// identified "sink" (for a given model).
type LogFwdLastSentID struct {
	// ModelTag identifies the model associated with the log record.
	ModelTag string `json:"model"`

	// Sink is the name of the log forwarding target to which a log
	// record was last sent.
	Sink string `json:"sink"`
}

// LogFwdLastSentGetParams holds the arguments for a bulk call to the
// Get method of the LogFwdLastSent facade.
type LogFwdLastSentGetParams struct {
	// IDs holds the list of IDs for which individual "last sent"
	// timestamps should be returned (in the same order).
	IDs []LogFwdLastSentID `json:"ids"`
}

// LogFwdLastSentGetResults holds the results of a bulk call to the
// Get method of the LogFwdLastSent facade.
type LogFwdLastSentGetResults struct {
	// Results holds the list of results that correspond to the IDs
	// sent in a bulkd Get call.
	Results []LogFwdLastSentGetResult `json:"results"`
}

// LogFwdLastSentGetResult holds a single result from a call to the
// Get method of the LogFwdLastSent facade.
type LogFwdLastSentGetResult struct {
	// Timestamp identifies the last log record that was forwarded
	// for a given model and sink. If Error is set then the meaning
	// of this value is undefined.
	//
	// Note that if more than one log record has the same timestamp
	// down to the nanosecond then the timestamp will not identify any
	// of them uniquely. However, the likelihood of such a collision
	// is remote (though it grows with more agents and more activity).
	Timestamp time.Time `json:"ts"`

	// Error holds the error, if any, that resulted while handling the
	// request for a specific ID.
	Error *Error `json:"err"`
}

// LogFwdLastSentSetParams holds the arguments for a bulk call to the
// Set method of the LogFwdLastSent facade.
type LogFwdLastSentSetParams struct {
	// Params holds the list of individual requests for "last sent" info.
	Params []LogFwdLastSentSetParam `json:"params"`
}

// LogFwdLastSentSetParams holds holds the info needed to set a new
// "last sent" value via a bulk call to the Set method of the
// LogFwdLastSent facade.
type LogFwdLastSentSetParam struct {
	LogFwdLastSentID

	// Timestamp identifies the timestamp to set for the given ID.
	Timestamp time.Time `json:"ts"`
}
