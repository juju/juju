package state

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// generationDoc represents the state of a model generation in MongoDB.
type generationDoc struct {
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

	// AssignedUnits is a map of unit IDs that are in the generation,
	// keyed by application ID.
	// An application ID can be present without any unit IDs,
	// which indicates that it has configuration changes applied in the
	// generation, but no units currently set to be in it.
	AssignedUnits map[string]string `bson:"assigned-units"`

	// Completed, if set, indicates when this generation was completed and
	// effectively became the current model generation.
	Completed int64 `bson:"updated"`
}

// Generation represents the state of a model generation.
type Generation struct {
	st  *State
	doc generationDoc
}

// ModelUUID returns the ID of the model to which this generation applies.
func (g *Generation) ModelUUID() string {
	return g.doc.ModelUUID
}

// Active indicates the whether the model generation is currently active.
// true:  active model generation = "next"
// false: active model generation = "current"
func (g *Generation) Active() bool {
	return g.doc.Active
}

// AssignedUnits returns the unit IDs, keyed by application ID
// that have been assigned to this generation.
func (g *Generation) AssignedUnits() map[string]string {
	return g.doc.AssignedUnits
}

// NextGeneration returns the model's "next" generation
// if one exists that is not yet completed.
func (m *Model) NextGeneration() (*Generation, error) {
	gen, err := m.st.NextGeneration(m.UUID())
	return gen, errors.Trace(err)
}

// NextGeneration returns the "next" generation
// if one exists for the input model, that is not yet completed.
func (st *State) NextGeneration(modelUUID string) (*Generation, error) {
	doc, err := st.getNextGenerationDoc(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newGeneration(st, doc), nil
}

func (st *State) getNextGenerationDoc(modelUUID string) (*generationDoc, error) {
	col, closer := st.db().GetCollection(generationsC)
	defer closer()

	var err error
	doc := &generationDoc{}

	err = col.Find(bson.D{
		{"model-uuid", modelUUID},
		{"completed", 0},
	}).One(doc)

	switch err {
	case nil:
		return doc, nil
	case mgo.ErrNotFound:
		return nil, errors.NotFoundf("active model generation for %s", modelUUID)
	default:
		return nil, errors.Annotatef(err, "retrieving active model generation for %s", modelUUID)
	}
}

func newGeneration(st *State, doc *generationDoc) *Generation {
	return &Generation{
		st:  st,
		doc: *doc,
	}
}

// AddGeneration creates a new "next" generation for the model.
func (m *Model) AddGeneration() error {
	return errors.Trace(m.st.AddGeneration(m.UUID()))
}

// AddGeneration creates a new "next" generation for the input model ID.
// The inserted generation is active, meaning the model's current generation
// becomes "next" immediately.
// A new generation can be added for a model that has an existing generation
// that is not completed.
func (st *State) AddGeneration(modelUUID string) error {
	if !names.IsValidModel(modelUUID) {
		return errors.Errorf("%q is not a valid model UUID", modelUUID)
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if _, err := st.NextGeneration(modelUUID); err != nil {
			if err != mgo.ErrNotFound {
				return nil, errors.Annotatef(err, "checking model %q for next generation", modelUUID)
			}
		} else {
			return nil, errors.Errorf("model %q has a next generation that is not completed", modelUUID)
		}
		return insertGenerationOps(modelUUID), nil
	}

	err := st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		logger.Errorf("cannot create new generation for model %q: %v", modelUUID, err)
	}
	return err
}

func insertGenerationOps(modelUUID string) []txn.Op {
	doc := &generationDoc{
		ModelUUID:     modelUUID,
		Active:        true,
		AssignedUnits: map[string]string{},
	}

	return []txn.Op{
		{
			C:      modelsC,
			Id:     modelUUID,
			Assert: txn.DocExists,
		},
		{
			C:      machineUpgradeSeriesLocksC,
			Insert: doc,
		},
	}
}
