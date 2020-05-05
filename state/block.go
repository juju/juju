// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/model"
)

// Customers and stakeholders want to be able to prevent accidental damage to their Juju deployments.
// To prevent running some operations, we want to have blocks that can be switched on/off.
type Block interface {
	// Id returns this block's id.
	Id() string

	// ModelUUID returns the model UUID associated with this block.
	ModelUUID() string

	// Tag returns tag for the entity that is being blocked
	Tag() (names.Tag, error)

	// Type returns block type
	Type() BlockType

	// Message returns explanation that accompanies this block.
	Message() string

	updateMessageOp(string) ([]txn.Op, error)
}

// BlockType specifies block type for enum benefit.
type BlockType int8

const (
	// DestroyBlock type identifies block that prevents model destruction.
	DestroyBlock BlockType = iota

	// RemoveBlock type identifies block that prevents
	// removal of machines, applications, units or relations.
	RemoveBlock

	// ChangeBlock type identifies block that prevents model changes such
	// as additions, modifications, removals of model entities.
	ChangeBlock
)

var (
	typeNames = map[BlockType]model.BlockType{
		DestroyBlock: model.BlockDestroy,
		RemoveBlock:  model.BlockRemove,
		ChangeBlock:  model.BlockChange,
	}
	blockMigrationValue = map[BlockType]string{
		DestroyBlock: "destroy-model",
		RemoveBlock:  "remove-object",
		ChangeBlock:  "all-changes",
	}
)

// AllTypes returns all supported block types.
func AllTypes() []BlockType {
	return []BlockType{
		DestroyBlock,
		RemoveBlock,
		ChangeBlock,
	}
}

// ToParams returns the type as model.BlockType.
func (t BlockType) ToParams() model.BlockType {
	if jujuBlock, ok := typeNames[t]; ok {
		return jujuBlock
	}
	panic(fmt.Sprintf("unknown block type %d", int(t)))
}

// String returns humanly readable type representation.
func (t BlockType) String() string {
	return string(t.ToParams())
}

// MigrationValue converts the block type value into a useful human readable
// string for model migration.
func (t BlockType) MigrationValue() string {
	if value, ok := blockMigrationValue[t]; ok {
		return value
	}
	return "unknown"
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

// blockDoc records information about an model block.
type blockDoc struct {
	DocID     string    `bson:"_id"`
	ModelUUID string    `bson:"model-uuid"`
	Tag       string    `bson:"tag"`
	Type      BlockType `bson:"type"`
	Message   string    `bson:"message,omitempty"`
}

func (b *block) updateMessageOp(message string) ([]txn.Op, error) {
	return []txn.Op{{
		C:      blocksC,
		Id:     b.doc.DocID,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"message", message}}}},
	}}, nil
}

// Id is part of the state.Block interface.
func (b *block) Id() string {
	return b.doc.DocID
}

// ModelUUID is part of the state.Block interface.
func (b *block) ModelUUID() string {
	return b.doc.ModelUUID
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
// current model.
func (st *State) SwitchBlockOn(t BlockType, msg string) error {
	return setModelBlock(st, t, msg)
}

// SwitchBlockOff disables block of specified type for the
// current model.
func (st *State) SwitchBlockOff(t BlockType) error {
	return RemoveModelBlock(st, t)
}

// GetBlockForType returns the Block of the specified type for the current model
// where
//     not found -> nil, false, nil
//     found -> block, true, nil
//     error -> nil, false, err
func (st *State) GetBlockForType(t BlockType) (Block, bool, error) {
	return getBlockForType(st, t)
}

func getBlockForType(mb modelBackend, t BlockType) (Block, bool, error) {
	all, closer := mb.db().GetCollection(blocksC)
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

// AllBlocks returns all blocks in the model.
func (st *State) AllBlocks() ([]Block, error) {
	blocksCollection, closer := st.db().GetCollection(blocksC)
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

// AllBlocksForController returns all blocks in any models on
// the controller.
func (st *State) AllBlocksForController() ([]Block, error) {
	blocksCollection, closer := st.db().GetRawCollection(blocksC)
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

// RemoveAllBlocksForController removes all the blocks for the controller.
// It does not prevent new blocks from being added during / after
// removal.
func (st *State) RemoveAllBlocksForController() error {
	blocks, err := st.AllBlocksForController()
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
	// multiple models.
	return st.runRawTransaction(ops)
}

// setModelBlock updates the blocks collection with the
// specified block.
// Only one instance of each block type can exist in model.
func setModelBlock(mb modelBackend, t BlockType, msg string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		block, exists, err := getBlockForType(mb, t)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// Cannot create blocks of the same type more than once per model.
		// Cannot update current blocks.
		if exists {
			return block.updateMessageOp(msg)
		}
		return createModelBlockOps(mb, t, msg)
	}
	return mb.db().Run(buildTxn)
}

// newBlockId returns a sequential block id for this model.
func newBlockId(mb modelBackend) (string, error) {
	seq, err := sequence(mb, "block")
	if err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprint(seq), nil
}

func createModelBlockOps(mb modelBackend, t BlockType, msg string) ([]txn.Op, error) {
	id, err := newBlockId(mb)
	if err != nil {
		return nil, errors.Annotatef(err, "getting new block id")
	}
	// NOTE: if at any time in the future, we change blocks so that the
	// Tag is different from the model, then the migration of blocks will
	// need to change format.
	newDoc := blockDoc{
		DocID:     mb.docID(id),
		ModelUUID: mb.modelUUID(),
		Tag:       names.NewModelTag(mb.modelUUID()).String(),
		Type:      t,
		Message:   msg,
	}
	insertOp := txn.Op{
		C:      blocksC,
		Id:     newDoc.DocID,
		Assert: txn.DocMissing,
		Insert: &newDoc,
	}
	return []txn.Op{insertOp}, nil
}

func RemoveModelBlock(st *State, t BlockType) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		return RemoveModelBlockOps(st, t)
	}
	return st.db().Run(buildTxn)
}

func RemoveModelBlockOps(st *State, t BlockType) ([]txn.Op, error) {
	tBlock, exists, err := st.GetBlockForType(t)
	if err != nil {
		return nil, errors.Annotatef(err, "removing block %v", t.String())
	}
	if exists {
		return []txn.Op{{
			C:      blocksC,
			Id:     tBlock.Id(),
			Remove: true,
		}}, nil
	}
	// If the block doesn't exist, we're all good.
	return nil, nil
}
