// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/state/multiwatcher"
)

type Block interface {
	// Environment returns environment UUID where this block is applied.
	Environment() string

	// Type returns block type
	Type() BlockType

	// Message returns explanation that accompanies this block.
	Message() string
}

// BlockType specifies block type for enum benefit.
type BlockType int8

const (
	// DestroyBlock type identifies block that prevents environment destruction.
	DestroyBlock BlockType = iota

	// RemoveBlock type identifies block that prevents
	// removal of machines, services, units or relations.
	RemoveBlock

	// ChangeBlock type identifies block that prevents environment changes such
	// as additions, modifications, removals of environment entities.
	ChangeBlock
)

var typeNames = map[BlockType]multiwatcher.BlockType{
	DestroyBlock: multiwatcher.BlockDestroy,
	RemoveBlock:  multiwatcher.BlockRemove,
	ChangeBlock:  multiwatcher.BlockChange,
}

// AllTypes returns all supported block types.
func AllTypes() []BlockType {
	return []BlockType{
		DestroyBlock,
		RemoveBlock,
		ChangeBlock,
	}
}

// ToParams returns the type as multiwatcher.BlockType.
func (t BlockType) ToParams() multiwatcher.BlockType {
	if jujuBlock, ok := typeNames[t]; ok {
		return jujuBlock
	}
	return multiwatcher.BlockType(fmt.Sprintf("<unknown block type %d>", int(t)))
}

// String returns humanly readable type representation.
func (t BlockType) String() string {
	return string(t.ToParams())
}

type block struct {
	doc blockDoc
}

// blockDoc records information about an environment block.
type blockDoc struct {
	DocID   string     `bson:"_id"`
	EnvUUID string     `bson:"env-uuid"`
	Type    *BlockType `bson:"type"`
	Message string     `bson:"message,omitempty"`
}

// Implementation for Block.Environment().
func (b *block) Environment() string {
	return b.doc.EnvUUID
}

// DocId returns block id.
func (b *block) DocId() string {
	return b.doc.DocID
}

// Implementation for Block.Message().
func (b *block) Message() string {
	return b.doc.Message
}

// Implementation for Block.Type().
func (b *block) Type() BlockType {
	return *b.doc.Type
}

// SwitchBlockOn enables block of specified type for the
// current environment.
func (st *State) SwitchBlockOn(t BlockType, msg string) error {
	return setEnvironmentBlock(st, t, msg)
}

// SwitchBlockOff disables block of specified type for the
// current environment.
func (st *State) SwitchBlockOff(t BlockType) error {
	return removeEnvironmentBlock(st, t)
}

// HasBlock returns the Block of the specified type for the current environment.
// Nil if this type of block is not switched on.
func (st *State) HasBlock(t BlockType) (Block, error) {
	blocks, err := getEnvironmentBlocks(st)
	if err != nil && err != mgo.ErrNotFound {
		return nil, errors.Trace(err)
	}
	for _, b := range blocks {
		if b.Type() == t {
			return b, nil
		}
	}
	return nil, nil
}

// getEnvironmentBlocks returns all of the blocks associated with the
// environment.
func getEnvironmentBlocks(st *State) ([]Block, error) {
	envUUID := st.EnvironUUID()
	sel := bson.D{{"env-uuid", envUUID}}
	all, closer := st.getCollection(blocksC)
	defer closer()

	var docs []blockDoc
	err := all.Find(sel).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	blocks := make([]Block, len(docs))
	for i, doc := range docs {
		blocks[i] = &block{doc}
	}
	return blocks, nil
}

// setEnvironmentBlock updates the blocks collection with the
// specified block.
// Only one instance of each block type can exist in environment.
func setEnvironmentBlock(st *State, t BlockType, msg string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		currentBlocks, err := getEnvironmentBlocks(st)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		// Cannot create blocks of the same type more than once per environment.
		// Cannot update current blocks.
		// TODO(2015-02-09 anastasiamac) May change when we implement blocks per entity.
		for _, aBlock := range currentBlocks {
			aType := aBlock.Type()
			if aType == t {
				return nil, errors.New("block is already ON")
			}
		}
		return createEnvironmentBlockOps(st, t, msg)
	}
	return st.run(buildTxn)
}

func createEnvironmentBlockOps(st *State, t BlockType, msg string) ([]txn.Op, error) {
	envUUID := st.EnvironUUID()
	newDoc := blockDoc{
		EnvUUID: envUUID,
		DocID:   st.docID(t.String()),
		Type:    &t,
		Message: msg,
	}
	insertOp := txn.Op{
		C:      blocksC,
		Id:     newDoc.DocID,
		Assert: txn.DocMissing,
		Insert: &newDoc,
	}
	return []txn.Op{insertOp}, nil
}

func removeEnvironmentBlock(st *State, t BlockType) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		return removeEnvironmentBlockOps(st, t)
	}
	return st.run(buildTxn)
}

func removeEnvironmentBlockOps(st *State, t BlockType) ([]txn.Op, error) {
	envUUID := st.EnvironUUID()
	sel := bson.D{{"env-uuid", envUUID}}
	all, closer := st.getCollection(blocksC)
	defer closer()

	iter := all.Find(sel).Select(bson.D{{"type", t}}).Iter()
	defer iter.Close()
	var ops []txn.Op
	var doc blockDoc
	for iter.Next(&doc) {
		ops = append(ops, txn.Op{
			C:      blocksC,
			Id:     doc.DocID,
			Remove: true,
		})
	}
	return ops, errors.Trace(iter.Close())
}
