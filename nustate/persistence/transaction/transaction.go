// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transaction

import "time"

// An interface implemented by things that can be added into a model transaction
type Element interface {
	TxnElement()
}

// A type that can be embedded in other objects to allow them to be appended to
// a model transaction in a type-safe way. It implements TxnElement.
type BuildingBlock struct{}

func (BuildingBlock) TxnElement() {}

// A ModelTxn is a collection of transaction Element that can be applied atomically.
type ModelTxn []Element

// Tracks information about an executed transaction
// TODO: make this a context.Context with an embeddable value?
type Context struct {
	// Number of attempts before the txn got applied.
	Attempt int

	// Misc stats and debugging information. For example:
	ElapsedTime time.Duration
}
