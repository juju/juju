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

	fullPayloads := make([]payload.FullPayloadInfo, 0, len(docs))
	for _, doc := range docs {
		p := doc.payload()
		fullPayloads = append(fullPayloads, p)
	}
	return fullPayloads, nil
}

// Track adds records for the payload to persistence. If the payload
// is already there then it is replaced with the new one.
func (pp Persistence) Track(pl payload.FullPayloadInfo) error {
	logger.Tracef("inserting %q - %#v", pl.Name, pl)
	txn := &upsertPayloadTxn{
		payload: pl,
	}
	if err := pp.txns.run(txn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// SetStatus updates the raw status for the identified payload in
// persistence. If the payload is not there then payload.ErrNotFound
// is returned.
func (pp Persistence) SetStatus(unit, name, status string) error {
	logger.Tracef("setting status for %q", name)
	txn := &setPayloadStatusTxn{
		unit:   unit,
		name:   name,
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
func (pp Persistence) List(unit string, names ...string) ([]payload.FullPayloadInfo, []string, error) {
	// TODO(ericsnow) Ensure that the unit is Alive?

	docs, missing, err := pp.queries.someUnitPayloads(unit, names)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	results := make([]payload.FullPayloadInfo, len(docs))
	for i, doc := range docs {
		results[i] = doc.payload()
	}
	return results, missing, nil
}

// ListAllForUnit builds the list of all payloads found in persistence.
func (pp Persistence) ListAllForUnit(unit string) ([]payload.FullPayloadInfo, error) {
	// TODO(ericsnow) Ensure that the unit is Alive?

	docs, err := pp.queries.unitPayloadsByName(unit)
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]payload.FullPayloadInfo, 0, len(docs))
	for _, doc := range docs {
		p := doc.payload()
		results = append(results, p)
	}
	return results, nil
}

// TODO(ericsnow) Add payloads to state/cleanup.go.

// TODO(ericsnow) How to ensure they are completely removed from state
// (when you factor in status stored in a separate collection)?

// Untrack removes all records associated with the identified payload
// from persistence. If the payload is not there then
// payload.ErrNotFound is returned.
func (pp Persistence) Untrack(unit, name string) error {
	txn := &removePayloadTxn{
		unit: unit,
		name: name,
	}
	if err := pp.txns.run(txn); err != nil {
		return errors.Trace(err)
	}
	return nil
}
