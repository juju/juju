// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// LogForwardingID is the API data that identifies a log forwarding
// "last sent" value. The controller has a mapping from a set of IDs
// to a timestamp (for each ID). The timestamp corresponds to the last
// log record that the specific log forwarding machinery sent to the
// identified "sink" (for a given model).
type LogForwardingID struct {
	// ModelTag identifies the model associated with the log record.
	ModelTag string `json:"model"`

	// Sink is the name of the log forwarding target to which a log
	// record was last sent.
	Sink string `json:"sink"`
}

// LogForwardingGetLastSentParams holds the arguments for a call
// to the GetLastSent method of the LogForwarding facade.
type LogForwardingGetLastSentParams struct {
	// IDs holds the list of IDs for which individual "last sent"
	// timestamps should be returned (in the same order).
	IDs []LogForwardingID `json:"ids"`
}

// LogForwardingGetLastSentResults holds the results of a call
// to the GetLastSent method of the LogForwarding facade.
type LogForwardingGetLastSentResults struct {
	// Results holds the list of results that correspond to the IDs
	// sent in a GetLastSent call.
	Results []LogForwardingGetLastSentResult `json:"results"`
}

// LogForwardingGetLastSentResult holds a single result from a call
// to the GetLastSent method of the LogForwarding facade.
type LogForwardingGetLastSentResult struct {
	// RecordID is the ID of the last log record that was
	// forwarded for a given model and sink. If Error is set then the
	// meaning of this value is undefined.
	RecordID int64 `json:"record-id"`

	// RecordTimestamp is the timestamp of the last log record that was
	// forwarded for a given model and sink. If Error is set then the
	// meaning of this value is undefined.
	RecordTimestamp int64 `json:"record-timestamp"`

	// Error holds the error, if any, that resulted while handling the
	// request for a specific ID.
	Error *Error `json:"err"`
}

// LogForwardingSetLastSentParams holds the arguments for a call
// to the SetLastSent method of the LogForwarding facade.
type LogForwardingSetLastSentParams struct {
	// Params holds the list of individual requests for "last sent" info.
	Params []LogForwardingSetLastSentParam `json:"params"`
}

// LogForwardingSetLastSentParams holds holds the info needed to set
// a new "last sent" value via a call to the SetLastSent method of the
// LogForwarding facade.
type LogForwardingSetLastSentParam struct {
	LogForwardingID

	// RecordID identifies the record ID to set for the given ID.
	RecordID int64 `json:"record-id"`

	// RecordTimestamp identifies the record timestamp to set for the given ID.
	RecordTimestamp int64 `json:"record-timestamp"`
}
