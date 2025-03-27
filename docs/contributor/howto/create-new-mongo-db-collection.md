# Create a new MongoDB collection
Sometimes developers need to store some new entities in Juju DB. This document provides key points for adding a new
collection to MongoDB.

# Define a new collection in Juju DB

All collections are represented in the package `state`. This package enables reading, observing, and changing the state
stored in MongoDB of a whole model managed by juju.

A developer can define a new collection in:  https://github.com/juju/juju/blob/main/state/allcollections.go

Example of a new collection definition:

```text
...
// ----------------------
newEntitiesC: {
   indexes: []mgo.Index{{
      Key: []string{"model-uuid", "unit-id"},
   }},
},
...
newEntitiesC = "newEntities"

```

# Define a new entity collection structure

Create a golang file in the state subfolder: `state/new_entites.go`

Add state shim to interact with the Juju global state in your API:

```text
// NewEntitiesState returns the new entities for the current state.
func (st *State) NewEntityState() *new EntityPersistence {
	return &newEntityPersistence{
		st: st,
	}
}
// serviceLocatorPersistence provides the persistence
// functionality for service locators.
type serviceLocatorPersistence struct {
	st *State
}

```

Define a logger to be able to write logs from your API:

```text
var neLogger = logger.Child("new-entity")

```

Define a `New Entity` structure:

```text
type NewEntity struct {
	st  *State
	doc newEntityDoc
}

```

Define the document structure for the new MongoDB collection:

```text
type newEntityDoc struct {
	DocId              string                 `bson:"_id"`
	Id                 int                    `bson:"id"`
	UnitId             string                 `bson:"unit-id"`
	Param1             string                 `bson:"param-1"`
	Param2             string                 `bson:"param-2"`
	OtherParams        map[string]interface{} `bson:"other-params"`
      ...
}

```

# Develop an API to manipulate collection entities

You then need to implement methods that will help you interact with new collection docs. Let’s define some simple CRUD
methods for ‘new entity’.

## Add a new doc

Here is an example of a way to add a new doc to a MongoDB collocation:

```text
// AddNewEntity creates a new entity record, which ...
func (ne *newEntityPersistence) AddNewEntity(args params.AddNewEntityParams) (*NewEntity, error) {
	id, err := sequenceWithMin(sp.st, "new-entity", 1)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer errors.DeferredAnnotatef(&err, "cannot add new entity %q", args.Name)

	model, err := ne.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	} else if model.Life() != Alive {
		return nil, errors.Errorf("model is no longer alive")
	}

	newEntityDoc := newEntityDoc{
		DocId:              fmt.Sprintf("%s.%s", args.Name, args.UnitId),
		Id:                 id,
		Param1:             args.Param1,
		Param2:             args.Param2,
		UnitId:             args.UnitId,
		Params:             args.Params,
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// model may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(sp.st); err != nil {
				return nil, errors.Trace(err)
			}
			return nil, errors.AlreadyExistsf("new entity name: %s unit-id: %s", args.Name, args.UnitId)
		}
		ops := []txn.Op{
			model.assertActiveOp(),
			{
				C:      newEntitiesC,
				Id:     newEntityDoc.DocId,
				Assert: txn.DocMissing,
				Insert: &newEntityDoc,
			},
		}
		return ops, nil
	}
	if err = sp.st.db().Run(buildTxn); err != nil {
		return nil, errors.Trace(err)
	}
	return &NewEntity{doc: newEntityDoc}, nil
}

```

## Remove a new doc

Here is an example of a way to remove a new doc from a MongoDB collocation:

```text
// RemoveNewEntities removes a service locator record
func (ne *newEntitiesPersistence) RemoveNewEntity(neId string) []txn.Op {
	op := txn.Op{
		C:      newEntitiesC,
		Id:     neId,
		Remove: true,
	}
	return []txn.Op{op}
}

```