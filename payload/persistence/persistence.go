// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/payload"
)

var logger = loggo.GetLogger("juju.payload.persistence")

// TODO(ericsnow) Store status in the status collection?

// TODO(ericsnow) Move PersistenceBase to the components package?

// PersistenceBase exposes the core persistence functionality needed
// for payloads.
type PersistenceBase interface {
	payloadsDBQueryer
	payloadsTxnRunner
}

// UnitPersistence exposes the high-level persistence functionality
// related to payloads in Juju.
type Persistence struct {
	queries payloadsQueries
	txns    payloadsTransactions
}

// NewPersistence wraps the "db" in a new Persistence.
func NewPersistence(db PersistenceBase) *Persistence {
	queries := payloadsQueries{
		querier: db,
	}
	return &Persistence{
		queries: queries,
		txns: payloadsTransactions{
			queries: queries,
			runner:  db,
		},
	}
}

// ListAll returns the list of all payloads in the model.
func (pp *Persistence) ListAll() ([]payload.FullPayloadInfo, error) {
	logger.Tracef("listing all payloads")

	docs, err := pp.queries.all("")
	if err != nil {
		return nil, errors.Trace(err)
	}

	var fullPayloads []payload.FullPayloadInfo
	for _, doc := range docs {
		p := doc.payload()
		fullPayloads = append(fullPayloads, p)
	}
	return fullPayloads, nil
}

// Track adds records for the payload to persistence. If the payload
// is already there then false gets returned (true if inserted).
// Existing records are not checked for consistency.
func (pp Persistence) Track(unit, stID string, pl payload.FullPayloadInfo) error {
	logger.Tracef("inserting %q - %#v", stID, pl)
	txn := &insertPayloadTxn{
		unit:    unit,
		stID:    stID,
		payload: pl,
	}
	if err := pp.txns.run(txn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// SetStatus updates the raw status for the identified payload in
// persistence. The return value corresponds to whether or not the
// record was found in persistence. Any other problem results in
// an error. The payload is not checked for inconsistent records.
func (pp Persistence) SetStatus(unit, stID, status string) error {
	logger.Tracef("setting status for %q", stID)
	txn := &setPayloadStatusTxn{
		unit:   unit,
		stID:   stID,
		status: status,
	}
	if err := pp.txns.run(txn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// List builds the list of payloads found in persistence which match
// the provided IDs. The lists of IDs with missing records is also
// returned.
func (pp Persistence) List(unit string, stIDs ...string) ([]payload.FullPayloadInfo, []string, error) {
	// TODO(ericsnow) Ensure that the unit is Alive?

	docs, missing, err := pp.queries.unitPayloads(unit, stIDs)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	var results []payload.FullPayloadInfo
	for _, stID := range stIDs {
		doc, ok := docs[stID]
		if !ok {
			continue
		}
		p := doc.payload()
		results = append(results, p)
	}
	return results, missing, nil
}

// ListAllForUnit builds the list of all payloads found in persistence.
// Inconsistent records result in errors.NotValid.
func (pp Persistence) ListAllForUnit(unit string) ([]payload.FullPayloadInfo, error) {
	// TODO(ericsnow) Ensure that the unit is Alive?

	docs, err := pp.queries.allByStateID(unit)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []payload.FullPayloadInfo
	for _, doc := range docs {
		p := doc.payload()
		results = append(results, p)
	}
	return results, nil
}

// LookUp returns the payload ID for the given name/rawID pair.
func (pp Persistence) LookUp(unit, name, rawID string) (string, error) {
	// TODO(ericsnow) This could be more efficient.

	docs, err := pp.queries.allByStateID(unit)
	if err != nil {
		return "", errors.Trace(err)
	}

	for id, doc := range docs {
		if doc.match(name, rawID) {
			return id, nil
		}
	}

	return "", errors.NotFoundf("payload for %s/%s", name, rawID)
}

// TODO(ericsnow) Add payloads to state/cleanup.go.

// TODO(ericsnow) How to ensure they are completely removed from state
// (when you factor in status stored in a separate collection)?

// Untrack removes all records associated with the identified payload
// from persistence. Also returned is whether or not the payload was
// found. If the records for the payload are not consistent then
// errors.NotValid is returned.
func (pp Persistence) Untrack(unit, stID string) error {
	txn := &removePayloadTxn{
		unit: unit,
		stID: stID,
	}
	if err := pp.txns.run(txn); err != nil {
		return errors.Trace(err)
	}
	return nil
}
