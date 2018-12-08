// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sort"
	"strconv"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/model"
)

// generationDoc represents the state of a model generation in MongoDB.
type generationDoc struct {
	Id string `bson:"generation-id"`

	// ModelUUID indicates the model to which this generation applies.
	ModelUUID string `bson:"model-uuid"`

	// Active indicates whether this generation is currently the
	// active one for the model.
	// If true, the current model generation is indicated as "next" and
	// configuration values applied to this generation are
	// represented in its assigned units.
	// If false, the current model generation is "current";
	// configuration changes are applied in the standard fashion
	// and apply as usual to units not yet in the generation.
	Active bool `bson:"active"`

	// AssignedUnits is a map of unit names that are in the generation,
	// keyed by application name.
	// An application ID can be present without any unit IDs,
	// which indicates that it has configuration changes applied in the
	// generation, but no units currently set to be in it.
	AssignedUnits map[string][]string `bson:"assigned-units"`

	// Completed, if set, indicates when this generation was completed and
	// effectively became the current model generation.
	Completed int64 `bson:"completed"`
}

// Generation represents the state of a model generation.
type Generation struct {
	st  *State
	doc generationDoc
}

// Id is unique ID for the generation within a model.
func (g *Generation) Id() string {
	return g.doc.Id
}

// ModelUUID returns the ID of the model to which this generation applies.
func (g *Generation) ModelUUID() string {
	return g.doc.ModelUUID
}

// Active indicates the whether the model generation is currently active.
func (g *Generation) Active() bool {
	return g.doc.Active
}

// AssignedUnits returns the unit names, keyed by application name
// that have been assigned to this generation.
func (g *Generation) AssignedUnits() map[string][]string {
	return g.doc.AssignedUnits
}

// AssignApplication indicates that the application with the input name has had
// changes in this generation.
func (g *Generation) AssignApplication(appName string) error {
	return errors.NotImplementedf("AssignApplication")
}

// AssignUnit indicates that the unit with the input name has had been added
// to this generation and should realise config changes applied to it.
func (g *Generation) AssignUnit(unitName string) error {
	return errors.NotImplementedf("AssignApplication")
}

// CanAutoComplete returns true if every application that has had configuration
// changes in this generation also has *all* of its units assigned to the
// generation.
func (g *Generation) CanAutoComplete() (bool, error) {
	can, err := g.canComplete(false)
	return can, errors.Trace(err)
}

// CanCancel returns true if every application that has had configuration
// changes in this generation has *all or none* of its units assigned to the
// generation.
func (g *Generation) CanCancel() (bool, error) {
	can, err := g.canComplete(true)
	return can, errors.Trace(err)
}

func (g *Generation) canComplete(allowEmpty bool) (bool, error) {
	// This will prevent CanAutoComplete from returning true when no config
	// changes have been made to the generation.
	if !allowEmpty && len(g.doc.AssignedUnits) == 0 {
		return false, nil
	}

	for app, units := range g.doc.AssignedUnits {
		if len(units) == 0 {
			if !allowEmpty {
				return false, nil
			}
			continue
		}

		allAppUnits, err := appUnitNames(g.st, app)
		if err != nil {
			return false, errors.Trace(err)
		}

		if len(units) != len(allAppUnits) {
			return false, nil
		}

		sort.Strings(units)
		sort.Strings(allAppUnits)
		for i, u := range units {
			if allAppUnits[i] != u {
				return false, nil
			}
		}
	}

	return true, nil
}

func appUnitNames(st *State, appId string) ([]string, error) {
	unitsCollection, closer := st.db().GetCollection(unitsC)
	defer closer()

	var docs []unitDoc
	err := unitsCollection.Find(bson.D{{"application", appId}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	names := make([]string, len(docs))
	for i, doc := range docs {
		names[i] = doc.Name
	}
	return names, nil
}

// AddGeneration creates a new "next" generation for the model.
func (m *Model) AddGeneration() error {
	return errors.Trace(m.st.AddGeneration())
}

// AddGeneration creates a new "next" generation for the current model.
// The inserted generation is active, meaning the model's current generation
// becomes "next" immediately.
// A new generation can not be added for a model that has an existing
// generation that is not completed.
func (st *State) AddGeneration() error {
	seq, err := sequence(st, "generation")
	if err != nil {
		return errors.Trace(err)
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if _, err := st.NextGeneration(); err != nil {
			if !errors.IsNotFound(err) {
				return nil, errors.Annotatef(err, "checking for next model generation")
			}
		} else {
			return nil, errors.Errorf("model has a next generation that is not completed")
		}

		return insertGenerationTxnOps(strconv.Itoa(seq)), nil
	}
	err = st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		logger.Errorf("cannot create new generation for model: %v", err)
	}
	return err
}

func insertGenerationTxnOps(id string) []txn.Op {
	doc := &generationDoc{
		Id:            id,
		Active:        true,
		AssignedUnits: map[string][]string{},
	}

	return []txn.Op{
		{
			C:      generationsC,
			Id:     id,
			Insert: doc,
		},
	}
}

// ActiveGeneration returns the active generation for the model.
func (m *Model) ActiveGeneration() (model.GenerationVersion, error) {
	v, err := m.st.ActiveGeneration()
	return v, errors.Trace(err)
}

// ActiveGeneration returns the active generation for the current model.
// If there is no "next" generation pending completion, or if the generation is
// not active, then the generation to use for config is "current".
// Otherwise, the model's "next" generation is active.
func (st *State) ActiveGeneration() (model.GenerationVersion, error) {
	gen, err := st.NextGeneration()
	if err != nil {
		if errors.IsNotFound(err) {
			return model.GenerationCurrent, nil
		}
		return "", errors.Trace(err)
	}

	if gen.Active() {
		return model.GenerationNext, nil
	}
	return model.GenerationCurrent, nil
}

// SwitchGeneration ensures that the active generation of the model matches the
// input version. This operation is idempotent
func (m *Model) SwitchGeneration(version model.GenerationVersion) error {
	return errors.Trace(m.st.SwitchGeneration(version))
}

// SwitchGeneration ensures that the active generation of the current model
// matches the input version. This operation is idempotent.
func (st *State) SwitchGeneration(version model.GenerationVersion) error {
	active := version == model.GenerationNext

	gen, err := st.NextGeneration()
	if err != nil {
		if errors.IsNotFound(err) && active {
			return errors.New("cannot switch to next generation, as none exists")
		}
		return errors.Trace(err)
	}

	if gen.Active() == active {
		return nil
	}
	gen.doc.Active = active

	buildTxn := func(attempt int) ([]txn.Op, error) {
		return switchGenerationTxnOps(gen), nil
	}
	err = st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		return err
	}
	return nil
}

func switchGenerationTxnOps(gen *Generation) []txn.Op {
	return []txn.Op{
		{
			C:      generationsC,
			Id:     gen.Id(),
			Update: bson.D{{"$set", bson.D{{"active", gen.Active()}}}},
		},
	}
}

// NextGeneration returns the model's "next" generation
// if one exists that is not yet completed.
func (m *Model) NextGeneration() (*Generation, error) {
	gen, err := m.st.NextGeneration()
	return gen, errors.Trace(err)
}

// NextGeneration returns the "next" generation
// if one exists for the current model, that is not yet completed.
func (st *State) NextGeneration() (*Generation, error) {
	doc, err := st.getNextGenerationDoc()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newGeneration(st, doc), nil
}

func (st *State) getNextGenerationDoc() (*generationDoc, error) {
	col, closer := st.db().GetCollection(generationsC)
	defer closer()

	var err error
	doc := &generationDoc{}
	err = col.Find(bson.D{{"completed", 0}}).One(doc)

	switch err {
	case nil:
		return doc, nil
	case mgo.ErrNotFound:
		mod, _ := st.modelName()
		return nil, errors.NotFoundf("next generation for %q", mod)
	default:
		mod, _ := st.modelName()
		return nil, errors.Annotatef(err, "retrieving next generation for %q", mod)
	}
}

func newGeneration(st *State, doc *generationDoc) *Generation {
	return &Generation{
		st:  st,
		doc: *doc,
	}
}
