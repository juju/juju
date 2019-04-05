// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/core/model"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// generationDoc represents the state of a model generation in MongoDB.
type generationDoc struct {
	DocId    string `bson:"_id"`
	TxnRevno int64  `bson:"txn-revno"`

	// Name is the name given to this branch at creation.
	// There should never be more than one branch, not applied or aborted,
	// with the same name. Branch names can otherwise be re-used.
	Name string `bson:"name"`

	// GenerationId is a monotonically incrementing sequence,
	// set when a branch is committed to the model.
	// Branches that are not applied, or that have been aborted
	// will not have a generation ID set.
	GenerationId int `bson:"generation-id"`

	// ModelUUID indicates the model to which this generation applies.
	ModelUUID string `bson:"model-uuid"`

	// AssignedUnits is a map of unit names that are in the generation,
	// keyed by application name.
	// An application ID can be present without any unit IDs,
	// which indicates that it has configuration changes applied in the
	// generation, but no units currently set to be in it.
	AssignedUnits map[string][]string `bson:"assigned-units"`

	// Config is all changes made to charm configuration under this branch.
	Config model.BranchCharmConfig `bson:"charm-config"`

	// TODO (manadart 2019-04-02): CharmURLs, Resources.

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

func (g *Generation) BranchName() string {
	return g.doc.Name
}

// GenerationId indicates the relative order that this branch was committed
// and had its changes applied to the whole model.
func (g *Generation) GenerationId() int {
	return g.doc.GenerationId
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

// Config returns all changed charm configuration for the generation.
func (g *Generation) Config() model.BranchCharmConfig {
	return g.doc.Config
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
		if err := g.CheckNotComplete(); err != nil {
			return nil, err
		}
		return assignGenerationAppTxnOps(g.doc.DocId, appName), nil
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

// AssignAllUnits ensures that all units of the input application are
// designated as tracking the branch, by adding the unit names
// to the generation.
func (g *Generation) AssignAllUnits(appName string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := g.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if err := g.CheckNotComplete(); err != nil {
			return nil, errors.Trace(err)
		}
		unitNames, err := appUnitNames(g.st, appName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		app, err := g.st.Application(appName)
		if err != nil {
			return nil, errors.Trace(err)
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
				unit, err := g.st.Unit(name)
				if err != nil {
					return nil, errors.Trace(err)
				}
				ops = append(ops, assignGenerationUnitTxnOps(g.doc.DocId, appName, unit)...)
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

// AssignUnit indicates that the unit with the input name is tracking this
// branch, by adding the name to the generation.
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
		if err := g.CheckNotComplete(); err != nil {
			return nil, errors.Trace(err)
		}
		if set.NewStrings(g.doc.AssignedUnits[appName]...).Contains(unitName) {
			return nil, jujutxn.ErrNoOperations
		}
		unit, err := g.st.Unit(unitName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return assignGenerationUnitTxnOps(g.doc.DocId, appName, unit), nil
	}

	return errors.Trace(g.st.db().Run(buildTxn))
}

func assignGenerationUnitTxnOps(id, appName string, unit *Unit) []txn.Op {
	assignedField := "assigned-units"
	appField := fmt.Sprintf("%s.%s", assignedField, appName)

	return []txn.Op{
		{
			C:      unitsC,
			Id:     unit.doc.DocID,
			Assert: bson.D{{"life", Alive}},
		},
		{
			C:  generationsC,
			Id: id,
			Assert: bson.D{{"$and", []bson.D{
				{{"completed", 0}},
				{{assignedField, bson.D{{"$exists", true}}}},
				{{appField, bson.D{{"$not", bson.D{{"$elemMatch", bson.D{{"$eq", unit.Name()}}}}}}}},
			}}},
			Update: bson.D{
				{"$push", bson.D{{appField, unit.Name()}}},
			},
		},
	}
}

// UpdateCharmConfig applies the input changes to the input application's
// charm configuration under this branch.
// the incoming charm settings are assumed to have been validated.
func (g *Generation) UpdateCharmConfig(appName string, master *Settings, validChanges charm.Settings) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := g.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if err := g.CheckNotComplete(); err != nil {
			return nil, errors.Trace(err)
		}

		if g.doc.Config == nil {
			g.doc.Config = make(model.BranchCharmConfig)
		}

		// Apply the current branch deltas to the master settings.
		branchDelta, branchHasDelta := g.doc.Config[appName]
		if branchHasDelta {
			master.applyChanges(branchDelta)
		}

		// Now apply the incoming changes on top.
		for k, v := range validChanges {
			if v == nil {
				master.Delete(k)
			} else {
				master.Set(k, v)
			}
		}

		// Now get the resulting delta.
		newDelta := master.changes()

		// Ensure that the old values from the delta always indicate their
		// original values.
		if branchHasDelta {
			if err := newDelta.ApplyDeltaSource(branchDelta); err != nil {
				return nil, errors.Trace(err)
			}
		}

		g.doc.Config[appName] = newDelta

		return []txn.Op{
			{
				C:  generationsC,
				Id: g.doc.DocId,
				Assert: bson.D{{"$and", []bson.D{
					{{"completed", 0}},
					{{"txn-revno", g.doc.TxnRevno}},
				}}},
				Update: bson.D{
					{"$set", bson.D{{"charm-config", g.doc.Config}}},
				},
			},
		}, nil
	}

	return errors.Trace(g.st.db().Run(buildTxn))
}

// Commit marks the generation as completed and assigns it the next value from
// the generation sequence. The new generation ID is returned.
func (g *Generation) Commit(userName string) (int, error) {
	var newGenId int

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := g.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if g.IsCompleted() {
			if g.GenerationId() == 0 {
				return nil, errors.New("branch was already aborted")
			}
			return nil, jujutxn.ErrNoOperations
		}
		now, err := g.st.ControllerTimestamp()
		if err != nil {
			return nil, errors.Trace(err)
		}

		// Add all units who's applications have changed.
		assigned := g.AssignedUnits()
		for app := range assigned {
			units, err := appUnitNames(g.st, app)
			if err != nil {
				return nil, errors.Trace(err)
			}
			assigned[app] = units
		}

		// Get the new sequence as late as we can.
		// If assigned is empty, indicating no changes under this branch,
		// then the generation ID in not incremented.
		// This effectively means the generation is aborted, not committed.
		if len(assigned) > 0 {
			id, err := sequenceWithMin(g.st, "generation", 1)
			if err != nil {
				return nil, errors.Trace(err)
			}
			newGenId = id
		}

		// As a proxy for checking that the generation has not changed,
		// Assert that the txn rev-no has not changed since we materialised
		// this generation object.
		ops := []txn.Op{
			{
				C:      generationsC,
				Id:     g.doc.DocId,
				Assert: bson.D{{"txn-revno", g.doc.TxnRevno}},
				Update: bson.D{
					{"$set", bson.D{
						{"assigned-units", assigned},
						{"completed", now.Unix()},
						{"completed-by", userName},
						{"generation-id", newGenId},
					}},
				},
			},
		}
		return ops, nil
	}

	if err := g.st.db().Run(buildTxn); err != nil {
		return 0, errors.Trace(err)
	}
	return newGenId, nil
}

// TODO (manadart 2019-03-19): Implement Abort().

// CheckNotComplete returns an error if this
// generation was committed or aborted.
func (g *Generation) CheckNotComplete() error {
	if g.doc.Completed == 0 {
		return nil
	}

	msg := "committed"
	if g.doc.GenerationId == 0 {
		msg = "aborted"
	}
	return errors.New("branch was already " + msg)
}

func appUnitNames(st *State, appName string) ([]string, error) {
	unitsCollection, closer := st.db().GetCollection(unitsC)
	defer closer()

	var docs []struct {
		Name string `bson:"name"`
	}
	err := unitsCollection.Find(bson.D{{"application", appName}}).Select(bson.D{{"name", 1}}).All(&docs)
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
	if err := col.FindId(g.doc.DocId).One(&doc); err != nil {
		return errors.Trace(err)
	}
	g.doc = doc
	return nil
}

// AddBranch creates a new branch in the current model.
func (m *Model) AddBranch(branchName, userName string) error {
	return errors.Trace(m.st.AddBranch(branchName, userName))
}

// AddBranch creates a new branch in the current model.
// A branch cannot be created with the same name as another "in-flight" branch.
// The input user indicates the operator who invoked the creation.
func (st *State) AddBranch(branchName, userName string) error {
	id, err := sequence(st, "branch")
	if err != nil {
		return errors.Trace(err)
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if _, err := st.Branch(branchName); err != nil {
			if !errors.IsNotFound(err) {
				return nil, errors.Annotatef(err, "checking for existing branch")
			}
		} else {
			return nil, errors.Errorf("model already has branch %q", branchName)
		}

		now, err := st.ControllerTimestamp()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return insertGenerationTxnOps(strconv.Itoa(id), branchName, userName, now), nil
	}
	err = st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		logger.Errorf("cannot add branch to the model: %v", err)
	}
	return err
}

func insertGenerationTxnOps(id, branchName, userName string, now *time.Time) []txn.Op {
	doc := &generationDoc{
		Name:          branchName,
		AssignedUnits: map[string][]string{},
		Created:       now.Unix(),
		CreatedBy:     userName,
	}

	return []txn.Op{
		{
			C:      generationsC,
			Id:     id,
			Insert: doc,
		},
	}
}

// Branch retrieves the generation with the the input branch name from the
// collection of not-yet-completed generations.
func (m *Model) Branch(name string) (*Generation, error) {
	gen, err := m.st.Branch(name)
	return gen, errors.Trace(err)
}

// Branch retrieves the generation with the the input branch name from the
// collection of not-yet-completed generations.
func (st *State) Branch(name string) (*Generation, error) {
	doc, err := st.getBranchDoc(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newGeneration(st, doc), nil
}

func (st *State) getBranchDoc(name string) (*generationDoc, error) {
	col, closer := st.db().GetCollection(generationsC)
	defer closer()

	var err error
	doc := &generationDoc{}
	err = col.Find(bson.M{
		"name":      name,
		"completed": 0,
	}).One(doc)

	switch err {
	case nil:
		return doc, nil
	case mgo.ErrNotFound:
		mod, _ := st.modelName()
		return nil, errors.NotFoundf("branch %q in model %q", name, mod)
	default:
		mod, _ := st.modelName()
		return nil, errors.Annotatef(err, "retrieving branch %q in model %q", name, mod)
	}
}

func newGeneration(st *State, doc *generationDoc) *Generation {
	return &Generation{
		st:  st,
		doc: *doc,
	}
}
