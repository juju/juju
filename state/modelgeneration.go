// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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

	// Created is a Unix timestamp indicating when this generation was created.
	Created int64 `bson:"created"`

	// CreatedBy is the user who created this generation.
	CreatedBy string `bson:"created-by"`

	// Completed, if set, indicates when this generation was completed and
	// effectively became the current model generation.
	Completed int64 `bson:"completed"`

	// CompletedBy is the user who committed this generation to the model.
	CompletedBy string `bson:"completed-by"`
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

// Created returns the Unix timestamp at generation creation.
func (g *Generation) Created() int64 {
	return g.doc.Created
}

// CreatedBy returns the user who created the generation.
func (g *Generation) CreatedBy() string {
	return g.doc.CreatedBy
}

// IsCompleted returns true if the generation has been completed;
// i.e it has a completion time-stamp.
func (g *Generation) IsCompleted() bool {
	return g.doc.Completed > 0
}

// CompletedBy returns the user who committed the generation.
func (g *Generation) CompletedBy() string {
	return g.doc.CompletedBy
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

// AutoComplete marks the generation as completed if there are no applications
// with changes in this generation that do not have all units advanced to the
// generation. It then becomes the "current" generation, and true is returned.
// If the criteria above are not met, the generation is not completed and
// false is returned.
func (g *Generation) AutoComplete() (bool, error) {
	completed, err := g.complete(false)
	return completed, errors.Trace(err)
}

// MakeCurrent marks the generation as completed if there are no applications
// with changes in this generation that do not have all units on the same
// generation, which can be either "current" of "next".
// This the operation invoked by an operator "cancelling" a generation.
// It then becomes the "current" generation.
func (g *Generation) MakeCurrent() error {
	_, err := g.complete(true)
	return errors.Trace(err)
}

// TODO (hml) 23-jan-2019
// When implementing change history, review to see if this is
// still the best course of action.
func (g *Generation) complete(allowEmpty bool) (bool, error) {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := g.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if g.IsCompleted() {
			return nil, jujutxn.ErrNoOperations
		}
		if err := g.checkCanMakeCurrent(allowEmpty); err != nil {
			return nil, errors.Trace(err)
		}
		now, err := g.st.ControllerTimestamp()
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
					{"$set", bson.D{{"completed", now.Unix()}}},
				},
			},
		}
		return ops, nil
	}

	err := g.st.db().Run(buildTxn)
	completed := true
	if err != nil {
		// if we are auto-completing, and criteria are not met, just return
		// false. This error is not relevant to MakeCurrent (cancel).
		if errors.Cause(err) == errGenerationNoAutoComplete && !allowEmpty {
			err = nil
		}
		completed = false
	}
	return completed, errors.Trace(err)
}

// checkCanMakeCurrent assesses the generation to determine whether it can be
// completed and rolled forward to become the "current" generation.
// The input boolean determines whether we permit applications with changes,
// but with no advanced units, to be deemed such candidates.
func (g *Generation) checkCanMakeCurrent(allowEmpty bool) error {
	// This will prevent AutoComplete from proceeding when no
	// changes have been made to the generation.
	if !allowEmpty && len(g.doc.AssignedUnits) == 0 {
		return errGenerationNoAutoComplete
	}

	unitsBehind := set.NewStrings()
	var appsWithoutUnitsFlag bool
	for app, units := range g.doc.AssignedUnits {
		if len(units) == 0 {
			if !allowEmpty {
				appsWithoutUnitsFlag = true
			}
			continue
		}

		allAppUnits, err := appUnitNames(g.st, app)
		if err != nil {
			return errors.Trace(err)
		}

		unitsSet := set.NewStrings(units...)
		allAppUnitsSet := set.NewStrings(allAppUnits...)

		diff := allAppUnitsSet.Difference(unitsSet)
		unitsBehind = unitsBehind.Union(diff)
	}

	if !unitsBehind.IsEmpty() || appsWithoutUnitsFlag {
		if allowEmpty {
			// This is the result of an operator attempting to cancel the
			// generation. Tell them which units are blocking the operation.
			return errors.Errorf("cannot cancel generation, there are units behind a generation: %s",
				strings.Join(unitsBehind.SortedValues(), ", "))
		} else {
			return errGenerationNoAutoComplete
		}
	}
	return nil
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
func (m *Model) AddGeneration(user string) error {
	return errors.Trace(m.st.AddGeneration(user))
}

// AddGeneration creates a new "next" generation for the current model.
// A new generation can not be added for a model that has an existing
// generation that is not completed.
// The input user indicates the operator who invoked the creation.
func (st *State) AddGeneration(user string) error {
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

		now, err := st.ControllerTimestamp()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return insertGenerationTxnOps(strconv.Itoa(seq), now, user), nil
	}
	err = st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		logger.Errorf("cannot create new generation for model: %v", err)
	}
	return err
}

func insertGenerationTxnOps(id string, now *time.Time, user string) []txn.Op {
	doc := &generationDoc{
		Id:            id,
		AssignedUnits: map[string][]string{},
		Created:       now.Unix(),
		CreatedBy:     user,
	}

	return []txn.Op{
		{
			C:      generationsC,
			Id:     id,
			Insert: doc,
		},
	}
}

// HasNextGeneration returns true if this model has a generation that has not
// yet been completed.
func (m *Model) HasNextGeneration() (bool, error) {
	_, err := m.NextGeneration()
	if err != nil {
		if errors.IsNotFound(err) {
			err = nil
		}
		return false, errors.Trace(err)
	}
	return true, nil
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

// errGenerationNoAutoComplete indicates that the generation can not be
// automatically completed. This error should never escape the package.
var errGenerationNoAutoComplete = errors.New("generation can not be auto-completed")
