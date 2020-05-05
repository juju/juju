// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/mongo/utils"
)

// itemChange is the state representation of a core settings ItemChange.
type itemChange struct {
	Type     int         `bson:"type"`
	Key      string      `bson:"key"`
	OldValue interface{} `bson:"old,omitempty"`
	NewValue interface{} `bson:"new,omitempty"`
}

// coreChange returns the core package representation of this change.
// Stored keys are unescaped.
func (c *itemChange) coreChange() settings.ItemChange {
	return settings.ItemChange{
		Type:     c.Type,
		Key:      utils.UnescapeKey(c.Key),
		OldValue: c.OldValue,
		NewValue: c.NewValue,
	}
}

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
	Config map[string][]itemChange `bson:"charm-config"`

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
// The persisted objects are converted to core changes.
func (g *Generation) Config() map[string]settings.ItemChanges {
	changes := make(map[string]settings.ItemChanges, len(g.doc.Config))
	for appName, appCfg := range g.doc.Config {
		appChanges := make(settings.ItemChanges, len(appCfg))
		for i, ch := range appCfg {
			appChanges[i] = ch.coreChange()
		}
		sort.Sort(appChanges)
		changes[appName] = appChanges
	}
	return changes
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

// Completed returns the Unix timestamp at generation completion.
func (g *Generation) Completed() int64 {
	return g.doc.Completed
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
	return g.AssignUnits(appName, 0)
}

func (g *Generation) AssignUnits(appName string, numUnits int) error {
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
		// Ensure we sort the unitNames so that when we ask for the numUnits
		// to track, they're going to be predictable results.
		sort.Strings(unitNames)

		var assigned int
		assignedUnits := set.NewStrings(g.doc.AssignedUnits[appName]...)
		for _, name := range unitNames {
			if !assignedUnits.Contains(name) {
				if numUnits > 0 && numUnits == assigned {
					break
				}
				unit, err := g.st.Unit(name)
				if err != nil {
					return nil, errors.Trace(err)
				}
				ops = append(ops, assignGenerationUnitTxnOps(g.doc.DocId, appName, unit)...)
				assigned++
			}
		}
		// If there are no units to add to the generation, quit here.
		if assigned == 0 {
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

		// Apply the current branch deltas to the master settings.
		branchChanges := g.Config()
		branchDelta, branchHasDelta := branchChanges[appName]
		if branchHasDelta {
			master.applyChanges(branchDelta)
		}

		// Now apply the incoming changes on top and generate a new delta.
		for k, v := range validChanges {
			if v == nil {
				master.Delete(k)
			} else {
				master.Set(k, v)
			}
		}
		newDelta := master.changes()

		// Ensure that the delta represents a change from master settings
		// as they were when each setting was first modified under the branch.
		if branchHasDelta {
			var err error
			if newDelta, err = newDelta.ApplyDeltaSource(branchDelta); err != nil {
				return nil, errors.Trace(err)
			}
		}

		return []txn.Op{
			{
				C:  generationsC,
				Id: g.doc.DocId,
				Assert: bson.D{{"$and", []bson.D{
					{{"completed", 0}},
					{{"txn-revno", g.doc.TxnRevno}},
				}}},
				Update: bson.D{
					{"$set", bson.D{{"charm-config." + appName, makeItemChanges(newDelta)}}},
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
		assigned, err := g.assignedWithAllUnits()
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops, err := g.commitConfigTxnOps()
		if err != nil {
			return nil, errors.Trace(err)
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
		ops = append(ops, txn.Op{
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
		})
		return ops, nil
	}

	if err := g.st.db().Run(buildTxn); err != nil {
		return 0, errors.Trace(err)
	}
	return newGenId, nil
}

// assignedWithAllUnits generates a new value for the branch's
// AssignedUnits field, to indicate that all units of changed applications
// are tracking the branch.
func (g *Generation) assignedWithAllUnits() (map[string][]string, error) {
	assigned := g.AssignedUnits()
	for app := range assigned {
		units, err := appUnitNames(g.st, app)
		if err != nil {
			return nil, errors.Trace(err)
		}
		assigned[app] = units
	}
	return assigned, nil
}

// commitConfigTxnOps iterates over all the applications with configuration
// deltas, determines their effective new settings, then gathers the
// operations representing the changes so that they can all be applied in a
// single transaction.
func (g *Generation) commitConfigTxnOps() ([]txn.Op, error) {
	var ops []txn.Op
	for appName, delta := range g.Config() {
		if len(delta) == 0 {
			continue
		}
		app, err := g.st.Application(appName)
		if err != nil {
			return nil, errors.Trace(err)
		}

		// Apply the branch delta to the application's charm config settings.
		cfg, err := readSettings(g.st.db(), settingsC, app.charmConfigKey())
		if err != nil {
			return nil, errors.Trace(err)
		}
		cfg.applyChanges(delta)

		_, updates := cfg.settingsUpdateOps()
		// Assert that the settings document has not changed underneath us
		// in addition to appending the field changes.
		if len(updates) > 0 {
			ops = append(ops, cfg.assertUnchangedOp())
			ops = append(ops, updates...)
		}
	}
	return ops, nil
}

// Abort marks the generation as completed however no value is assigned from
// the generation sequence.
func (g *Generation) Abort(userName string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := g.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		if g.IsCompleted() {
			if g.GenerationId() > 0 {
				return nil, errors.New("branch was already committed")
			}
			return nil, jujutxn.ErrNoOperations
		}

		// Must have no assigned units.
		assigned := g.AssignedUnits()
		for _, units := range assigned {
			if len(units) > 0 {
				return nil, errors.New("branch is in progress. Either reset values on tracking units and commit the branch or remove them to abort.")
			}
		}

		// Must not have upgraded charm of tracked application.
		// TODO (hml) 2019-06-26
		// Implement cannot abort branch where tracked application has
		// been upgraded.

		now, err := g.st.ControllerTimestamp()
		if err != nil {
			return nil, errors.Trace(err)
		}
		// As a proxy for checking that the generation has not changed,
		// Assert that the txn rev-no has not changed since we materialised
		// this generation object.
		ops := []txn.Op{{
			C:      generationsC,
			Id:     g.doc.DocId,
			Assert: bson.D{{"txn-revno", g.doc.TxnRevno}},
			Update: bson.D{
				{"$set", bson.D{
					{"completed", now.Unix()},
					{"completed-by", userName},
				}},
			},
		}}
		return ops, nil
	}

	return errors.Trace(g.st.db().Run(buildTxn))
}

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

// IsTracking returns true if the generation is tracking the provided unit.
func (g *Generation) IsTracking(unitName string) bool {
	var tracked bool
	for _, v := range g.doc.AssignedUnits {
		if tracked = set.NewStrings(v...).Contains(unitName); tracked {
			break
		}
	}
	return tracked
}

func (g *Generation) unassignUnitOps(unitName, appName string) []txn.Op {
	assignedField := "assigned-units"
	appField := fmt.Sprintf("%s.%s", assignedField, appName)

	// As a proxy for checking that the generation has not changed,
	// Assert that the txn rev-no has not changed since we materialised
	// this generation object.
	return []txn.Op{{
		C:      generationsC,
		Id:     g.doc.DocId,
		Assert: bson.D{{"txn-revno", g.doc.TxnRevno}},
		Update: bson.D{
			{"$pull", bson.D{{appField, unitName}}},
		},
	}}
}

// HasChangesFor returns true when the generation has config changes for
// the provided application.
func (g *Generation) HasChangesFor(appName string) bool {
	_, ok := g.doc.Config[appName]
	return ok
}

// unassignAppOps returns operations to remove the tracking and config data
// for the application from the generation.
func (g *Generation) unassignAppOps(appName string) []txn.Op {
	assigned := g.doc.AssignedUnits
	delete(assigned, appName)
	ops := []txn.Op{{
		C:      generationsC,
		Id:     g.doc.DocId,
		Assert: bson.D{{"txn-revno", g.doc.TxnRevno}},
		Update: bson.D{
			{"$set", bson.D{{"assigned-units", assigned}}},
		},
	}}
	currentCfg := g.doc.Config
	if _, ok := currentCfg[appName]; ok {
		newCfg := map[string][]itemChange{}
		for app, cfg := range currentCfg {
			if app == appName {
				continue
			}
			newCfg[app] = cfg
		}
		ops = append(ops, txn.Op{
			C:      generationsC,
			Id:     g.doc.DocId,
			Assert: bson.D{{"txn-revno", g.doc.TxnRevno}},
			Update: bson.D{
				{"$set", bson.D{{"charm-config", newCfg}}},
			},
		})
	}
	return ops
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

// Generations returns all committed  branches.
func (m *Model) Generations() ([]*Generation, error) {
	b, err := m.st.CommittedBranches()
	return b, errors.Trace(err)
}

// Branches returns all "in-flight" branches for the model.
func (m *Model) Branches() ([]*Generation, error) {
	b, err := m.st.Branches()
	return b, errors.Trace(err)
}

// Branches returns all "in-flight" branches.
func (st *State) Branches() ([]*Generation, error) {
	col, closer := st.db().GetCollection(generationsC)
	defer closer()

	var docs []generationDoc
	if err := col.Find(bson.M{"completed": 0}).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}

	branches := make([]*Generation, len(docs))
	for i, d := range docs {
		branches[i] = newGeneration(st, &d)
	}
	return branches, nil
}

// Generations returns all committed branches.
func (st *State) CommittedBranches() ([]*Generation, error) {
	col, closer := st.db().GetCollection(generationsC)
	defer closer()

	var docs []generationDoc
	query := bson.M{"generation-id": bson.M{"$gte": 1}}
	if err := col.Find(query).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}

	branches := make([]*Generation, len(docs))
	for i, d := range docs {
		branches[i] = newGeneration(st, &d)
	}
	return branches, nil
}

// Branch retrieves the generation with the the input branch name from the
// collection of not-yet-completed generations.
func (m *Model) Branch(name string) (*Generation, error) {
	gen, err := m.st.Branch(name)
	return gen, errors.Trace(err)
}

// Generation retrieves the generation with the the input generation_id from the
// collection of completed generations.
func (m *Model) Generation(id int) (*Generation, error) {
	gen, err := m.st.CommittedBranch(id)
	return gen, errors.Trace(err)
}

func (m *Model) applicationBranches(appName string) ([]*Generation, error) {
	branches, err := m.Branches()
	if err != nil {
		return nil, errors.Trace(err)
	}
	foundBranches := make([]*Generation, 0)
	for _, branch := range branches {
		if branch.HasChangesFor(appName) {
			foundBranches = append(foundBranches, branch)
			continue
		}
		if _, ok := branch.doc.AssignedUnits[appName]; ok {
			foundBranches = append(foundBranches, branch)
		}
	}
	return foundBranches, nil
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

// Generation retrieves the generation with the the input id from the
// collection of completed generations.
func (st *State) CommittedBranch(id int) (*Generation, error) {
	doc, err := st.getCommittedBranchDoc(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newGeneration(st, doc), nil
}

func (st *State) getBranchDoc(name string) (*generationDoc, error) {
	col, closer := st.db().GetCollection(generationsC)
	defer closer()

	doc := &generationDoc{}
	err := col.Find(bson.M{
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

func (st *State) getCommittedBranchDoc(id int) (*generationDoc, error) {
	col, closer := st.db().GetCollection(generationsC)
	defer closer()

	doc := &generationDoc{}
	err := col.Find(bson.M{
		"generation-id": id,
	}).One(doc)

	switch err {
	case nil:
		return doc, nil
	case mgo.ErrNotFound:
		mod, _ := st.modelName()
		return nil, errors.NotFoundf("generation_id %d in model %q", id, mod)
	default:
		mod, _ := st.modelName()
		return nil, errors.Annotatef(err, "retrieving generation_id %q in model %q", id, mod)
	}
}

func (m *Model) unitBranch(unitName string) (*Generation, error) {
	// NOTE (hml) 2019-07-02
	// Currently a unit may only be tracked in a single generation.
	// The branches spec indicates that may change in the future.  If
	// it does, this method and caller will need to be updated accordingly.
	branches, err := m.Branches()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, b := range branches {
		if b.IsTracking(unitName) {
			return b, nil
		}
	}
	return nil, nil
}

func newGeneration(st *State, doc *generationDoc) *Generation {
	return &Generation{
		st:  st,
		doc: *doc,
	}
}

// makeItemChanges generates a persistable collection of changes from a core
// settings representation, with keys escaped for Mongo.
func makeItemChanges(coreChanges settings.ItemChanges) []itemChange {
	changes := make([]itemChange, len(coreChanges))
	for i, c := range coreChanges {
		changes[i] = itemChange{
			Type:     c.Type,
			Key:      utils.EscapeKey(c.Key),
			OldValue: c.OldValue,
			NewValue: c.NewValue,
		}
	}
	return changes
}

// branchesCleanupChange removes the generation doc.
type branchesCleanupChange struct{}

// Prepare is part of the Change interface.
func (change branchesCleanupChange) Prepare(db Database) ([]txn.Op, error) {
	generations, closer := db.GetCollection(generationsC)
	defer closer()

	var docs []struct {
		DocID string `bson:"_id"`
	}
	err := generations.Find(nil).Select(bson.D{{"_id", 1}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(docs) == 0 {
		return nil, ErrChangeComplete
	}

	ops := make([]txn.Op, len(docs))
	for i, doc := range docs {
		ops[i] = txn.Op{
			C:      generationsC,
			Id:     doc.DocID,
			Remove: true,
		}
	}
	return ops, nil

}
