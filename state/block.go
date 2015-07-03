// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/state/multiwatcher"
)

// Customers and stakeholders want to be able to prevent accidental damage to their Juju deployments.
// To prevent running some operations, we want to have blocks that can be switched on/off.
type Block interface {
	// Id returns this block's id.
	Id() string

	// EnvUUID returns the environment UUID associated with this block.
	EnvUUID() string

	// Tag returns tag for the entity that is being blocked
	Tag() (names.Tag, error)

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
	panic(fmt.Sprintf("unknown block type %d", int(t)))
}

// String returns humanly readable type representation.
func (t BlockType) String() string {
	return string(t.ToParams())
}

// ParseBlockType returns BlockType from humanly readable type representation.
func ParseBlockType(str string) BlockType {
	for _, one := range AllTypes() {
		if one.String() == str {
			return one
		}
	}
	panic(fmt.Sprintf("unknown block type %v", str))
}

type block struct {
	doc blockDoc
}

// blockDoc records information about an environment block.
type blockDoc struct {
	DocID   string    `bson:"_id"`
	EnvUUID string    `bson:"env-uuid"`
	Tag     string    `bson:"tag"`
	Type    BlockType `bson:"type"`
	Message string    `bson:"message,omitempty"`
}

// Id is part of the state.Block interface.
func (b *block) Id() string {
	return b.doc.DocID
}

// EnvUUID is part of the state.Block interface.
func (b *block) EnvUUID() string {
	return b.doc.EnvUUID
}

// Message is part of the state.Block interface.
func (b *block) Message() string {
	return b.doc.Message
}

// Tag is part of the state.Block interface.
func (b *block) Tag() (names.Tag, error) {
	tag, err := names.ParseTag(b.doc.Tag)
	if err != nil {
		return nil, errors.Annotatef(err, "getting block information")
	}
	return tag, nil
}

// Type is part of the state.Block interface.
func (b *block) Type() BlockType {
	return b.doc.Type
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

// GetBlockForType returns the Block of the specified type for the current environment
// where
//     not found -> nil, false, nil
//     found -> block, true, nil
//     error -> nil, false, err
func (st *State) GetBlockForType(t BlockType) (Block, bool, error) {
	all, closer := st.getCollection(blocksC)
	defer closer()

	doc := blockDoc{}
	err := all.Find(bson.D{{"type", t}}).One(&doc)

	switch err {
	case nil:
		return &block{doc}, true, nil
	case mgo.ErrNotFound:
		return nil, false, nil
	default:
		return nil, false, errors.Annotatef(err, "cannot get block of type %v", t.String())
	}
}

// AllBlocks returns all blocks in the environment.
func (st *State) AllBlocks() ([]Block, error) {
	blocksCollection, closer := st.getCollection(blocksC)
	defer closer()

	var bdocs []blockDoc
	err := blocksCollection.Find(nil).All(&bdocs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all blocks")
	}
	blocks := make([]Block, len(bdocs))
	for i, doc := range bdocs {
		blocks[i] = &block{doc}
	}
	return blocks, nil
}

// AllBlocksForSystem returns all blocks in any environments on
// the system.
func (st *State) AllBlocksForSystem() ([]Block, error) {
	blocksCollection, closer := st.getRawCollection(blocksC)
	defer closer()

	var bdocs []blockDoc
	err := blocksCollection.Find(nil).All(&bdocs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all blocks")
	}
	blocks := make([]Block, len(bdocs))
	for i, doc := range bdocs {
		blocks[i] = &block{doc}
	}

	return blocks, nil
}

// RemoveAllBlocksForSystem removes all the blocks for the system.
// It does not prevent new blocks from being added during / after
// removal.
func (st *State) RemoveAllBlocksForSystem() error {
	blocks, err := st.AllBlocksForSystem()
	if err != nil {
		return errors.Trace(err)
	}

	ops := []txn.Op{}
	for _, blk := range blocks {
		ops = append(ops, txn.Op{
			C:      blocksC,
			Id:     blk.Id(),
			Remove: true,
		})
	}

	// Use runRawTransaction as we might be removing docs across
	// multiple environments.
	return st.runRawTransaction(ops)
}

// setEnvironmentBlock updates the blocks collection with the
// specified block.
// Only one instance of each block type can exist in environment.
func setEnvironmentBlock(st *State, t BlockType, msg string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		_, exists, err := st.GetBlockForType(t)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// Cannot create blocks of the same type more than once per environment.
		// Cannot update current blocks.
		if exists {
			return nil, errors.Errorf("block %v is already ON", t.String())
		}
		return createEnvironmentBlockOps(st, t, msg)
	}
	return st.run(buildTxn)
}

// newBlockId returns a sequential block id for this environment.
func newBlockId(st *State) (string, error) {
	seq, err := st.sequence("block")
	if err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprint(seq), nil
}

func createEnvironmentBlockOps(st *State, t BlockType, msg string) ([]txn.Op, error) {
	id, err := newBlockId(st)
	if err != nil {
		return nil, errors.Annotatef(err, "getting new block id")
	}
	newDoc := blockDoc{
		DocID:   st.docID(id),
		EnvUUID: st.EnvironUUID(),
		Tag:     st.EnvironTag().String(),
		Type:    t,
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
	tBlock, exists, err := st.GetBlockForType(t)
	if err != nil {
		return nil, errors.Annotatef(err, "removing block %v", t.String())
	}
	if exists {
		return []txn.Op{txn.Op{
			C:      blocksC,
			Id:     tBlock.Id(),
			Remove: true,
		}}, nil
	}
	return nil, errors.Errorf("block %v is already OFF", t.String())
}
