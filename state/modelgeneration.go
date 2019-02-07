// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// generationDoc represents the state of a model generation in MongoDB.
type generationDoc struct {
	Id       string `bson:"generation-id"`
	TxnRevno int64  `bson:"txn-revno"`

	// ModelUUID indicates the model to which this generation applies.
	ModelUUID string `bson:"model-uuid"`

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

// AssignedUnits returns the unit names, keyed by application name
// that have been assigned to this generation.
func (g *Generation) AssignedUnits() map[string][]string {
	return g.doc.AssignedUnits
}

func (g *Generation) IsCompleted() bool {
	return g.doc.Completed > 0
}

// AssignApplication indicates that the application with the input name has had
// changes in this generation.
func (g *Generation) AssignApplication(appName string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := g.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if _, ok := g.doc.AssignedUnits[appName]; ok {
			return nil, jujutxn.ErrNoOperations
		}
		if g.IsCompleted() {
			return nil, errors.New("generation has been completed")
		}
		return assignGenerationAppTxnOps(g.doc.Id, appName), nil
	}

	return errors.Trace(g.st.db().Run(buildTxn))
}

func assignGenerationAppTxnOps(id, appName string) []txn.Op {
	assignedField := "assigned-units"
	appField := fmt.Sprintf("%s.%s", assignedField, appName)

	return []txn.Op{
		{
			C:  generationsC,
			Id: id,
			Assert: bson.D{{"$and", []bson.D{
				{{"completed", 0}},
				{{assignedField, bson.D{{"$exists", true}}}},
				{{appField, bson.D{{"$exists", false}}}},
			}}},
			Update: bson.D{
				{"$set", bson.D{{appField, []string{}}}},
			},
		},
	}
}

// AssignAllUnits indicates that all units of the given application,
// not already added to this generation will be.
func (g *Generation) AssignAllUnits(appName string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := g.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if g.IsCompleted() {
			return nil, errors.New("generation has been completed")
		}
		unitNames, err := appUnitNames(g.st, appName)
		if err != nil {
			return nil, err
		}
		app, err := g.st.Application(appName)
		if err != nil {
			return nil, err
		}
		ops := []txn.Op{
			{
				C:  applicationsC,
				Id: app.doc.DocID,
				Assert: bson.D{
					{"life", Alive},
					{"unitcount", app.doc.UnitCount},
				},
			},
		}
		assignedUnits := set.NewStrings(g.doc.AssignedUnits[appName]...)
		for _, name := range unitNames {
			if !assignedUnits.Contains(name) {
				ops = append(ops, assignGenerationUnitTxnOps(g.doc.Id, appName, name)...)
			}
		}
		// If there are no units to add to the generation, quit here.
		if len(ops) < 2 {
			return nil, jujutxn.ErrNoOperations
		}
		return ops, nil
	}
	return errors.Trace(g.st.db().Run(buildTxn))
}

// AssignUnit indicates that the unit with the input name has had been added
// to this generation and should realise config changes applied to its
// application against this generation.
func (g *Generation) AssignUnit(unitName string) error {
	appName, err := names.UnitApplication(unitName)
	if err != nil {
		return errors.Trace(err)
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := g.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if g.IsCompleted() {
			return nil, errors.New("generation has been completed")
		}
		if set.NewStrings(g.doc.AssignedUnits[appName]...).Contains(unitName) {
			return nil, jujutxn.ErrNoOperations
		}
		return assignGenerationUnitTxnOps(g.doc.Id, appName, unitName), nil
	}

	return errors.Trace(g.st.db().Run(buildTxn))
}

func assignGenerationUnitTxnOps(id, appName, unitName string) []txn.Op {
	assignedField := "assigned-units"
	appField := fmt.Sprintf("%s.%s", assignedField, appName)

	return []txn.Op{
		{
			C:  generationsC,
			Id: id,
			Assert: bson.D{{"$and", []bson.D{
				{{"completed", 0}},
				{{assignedField, bson.D{{"$exists", true}}}},
				{{appField, bson.D{{"$not", bson.D{{"$elemMatch", bson.D{{"$eq", unitName}}}}}}}},
			}}},
			Update: bson.D{
				{"$push", bson.D{{appField, unitName}}},
			},
		},
	}
}

// AutoComplete marks the generation as completed if autocomplete criteria
// is met.  It then becomes the "current" generation.
func (g *Generation) AutoComplete() error {
	err := g.complete(false)
	return errors.Trace(err)
}

// AutoComplete marks the generation as completed if makecurrent criteria
// is met.  It then becomes the "current" generation.
func (g *Generation) MakeCurrent() error {
	err := g.complete(true)
	return errors.Trace(err)
}

// TODO (hml) 23-jan-2019
// When implementing change history, review to see if this is
// still the best course of action.
func (g *Generation) complete(allowEmpty bool) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := g.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if g.IsCompleted() {
			return nil, jujutxn.ErrNoOperations
		}
		ok, err := g.allowMakeCurrent(allowEmpty)
		if err != nil || !ok {
			return nil, errors.Trace(err)
		}
		time, err := g.st.ControllerTimestamp()
		if err != nil {
			return nil, errors.Trace(err)
		}
		// The generation doc has only 3 elements that change as part of juju
		// functionality.  We want to assert than none of those have changed
		// here, however ensuring that no new applications are added to
		// AssignedUnits, is non trivial.  Therefore just check the txn-revno
		// instead.
		ops := []txn.Op{
			{
				C:      generationsC,
				Id:     g.doc.Id,
				Assert: bson.D{{"txn-revno", g.doc.TxnRevno}},
				Update: bson.D{
					{"$set", bson.D{{"completed", time.Unix()}}},
				},
			},
		}
		return ops, nil
	}
	return errors.Trace(g.st.db().Run(buildTxn))
}

func (g *Generation) allowMakeCurrent(allowEmpty bool) (bool, error) {
	ok, values, err := g.canMakeCurrent(allowEmpty)
	if err != nil {
		return false, errors.Trace(err)
	}
	if !ok {
		if allowEmpty {
			return false, errors.Errorf(
				"cannot cancel generation, there are units behind a generation: %s", strings.Join(values, ", "))
		} else {
			return false, errors.New("generation can not be completed")
		}
	}
	return true, nil
}

// CanAutoComplete returns true if every application that has had configuration
// changes in this generation also has *all* of its units assigned to the
// generation.
// autocomplete, advance
func (g *Generation) CanAutoComplete() (bool, error) {
	can, _, err := g.canMakeCurrent(false)
	return can, errors.Trace(err)
}

// CanMakeCurrent returns true if every application that has had configuration
// changes in this generation has *all or none* of its units assigned to the
// generation.
// makecurrent, cancel.
func (g *Generation) CanMakeCurrent() (bool, []string, error) {
	can, units, err := g.canMakeCurrent(true)
	return can, units, errors.Trace(err)
}

func (g *Generation) canMakeCurrent(allowEmpty bool) (bool, []string, error) {
	// This will prevent CanAutoComplete from returning true when no config
	// changes have been made to the generation.
	if !allowEmpty && len(g.doc.AssignedUnits) == 0 {
		return false, nil, nil
	}

	cancel := set.NewStrings()
	var haveEmpty bool
	for app, units := range g.doc.AssignedUnits {
		if len(units) == 0 {
			if !allowEmpty {
				haveEmpty = true
			}
			continue
		}

		allAppUnits, err := appUnitNames(g.st, app)
		if err != nil {
			return false, nil, errors.Trace(err)
		}

		unitsSet := set.NewStrings(units...)
		allAppUnitsSet := set.NewStrings(allAppUnits...)

		diff := allAppUnitsSet.Difference(unitsSet)
		cancel = cancel.Union(diff)
	}

	return cancel.IsEmpty() && !haveEmpty, cancel.SortedValues(), nil
}

func appUnitNames(st *State, appId string) ([]string, error) {
	unitsCollection, closer := st.db().GetCollection(unitsC)
	defer closer()

	var docs []unitDoc
	err := unitsCollection.Find(bson.D{{"application", appId}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	unitNames := make([]string, len(docs))
	for i, doc := range docs {
		unitNames[i] = doc.Name
	}
	return unitNames, nil
}

// Refresh refreshes the contents of the generation from the underlying state.
func (g *Generation) Refresh() error {
	col, closer := g.st.db().GetCollection(generationsC)
	defer closer()

	var doc generationDoc
	if err := col.FindId(g.doc.Id).One(&doc); err != nil {
		return errors.Trace(err)
	}
	g.doc = doc
	return nil
}

// AddGeneration creates a new "next" generation for the model.
func (m *Model) AddGeneration() error {
	return errors.Trace(m.st.AddGeneration())
}

// AddGeneration creates a new "next" generation for the current model.
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
