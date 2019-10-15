// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/mongo/utils"
)

// defaultEndpointName is the key in the bindings map that stores the
// space name that endpoints should be bound to if they aren't found
// individually.
const defaultEndpointName = ""

// endpointBindingsDoc represents how a application's endpoints are bound to spaces.
// The DocID field contains the applications's global key, so there is always one
// endpointBindingsDoc per application.
type endpointBindingsDoc struct {
	// DocID is always the same as a application's global key.
	DocID string `bson:"_id"`

	// Bindings maps an application endpoint name to the space ID it is bound to.
	Bindings bindingsMap `bson:"bindings"`

	// TxnRevno is used to assert the collection have not changed since this
	// document was fetched.
	TxnRevno int64 `bson:"txn-revno"`
}

// bindingsMap is the underlying type stored in mongo for bindings.
type bindingsMap map[string]string

// SetBSON ensures any special characters ($ or .) are unescaped in keys after
// unmarshalling the raw BSON coming from the stored document.
func (b *bindingsMap) SetBSON(raw bson.Raw) error {
	rawMap := make(map[string]string)
	if err := raw.Unmarshal(rawMap); err != nil {
		return err
	}
	for key, value := range rawMap {
		newKey := utils.UnescapeKey(key)
		if newKey != key {
			delete(rawMap, key)
		}
		rawMap[newKey] = value
	}
	*b = bindingsMap(rawMap)
	return nil
}

// GetBSON ensures any special characters ($ or .) are escaped in keys before
// marshalling the map into BSON and storing in mongo.
func (b bindingsMap) GetBSON() (interface{}, error) {
	if b == nil || len(b) == 0 {
		// We need to return a non-nil map otherwise bson.Unmarshal
		// call will fail when reading the doc back.
		return make(map[string]string), nil
	}
	rawMap := make(map[string]string, len(b))
	for key, value := range b {
		newKey := utils.EscapeKey(key)
		rawMap[newKey] = value
	}

	return rawMap, nil
}

// mergeBindings returns the effective bindings, by combining the default
// bindings based on the given charm metadata, overriding them first with
// matching oldMap values, and then with newMap values (for the same keys).
// newMap and oldMap are both optional and will ignored when empty. Returns a
// map containing the combined finalized bindings.
// Returns true/false if there are any actual differences.
func mergeBindings(newMap, oldMap map[string]string, meta *charm.Meta) (map[string]string, bool, error) {
	defaultsMap := DefaultEndpointBindingsForCharm(meta)
	defaultBinding, oldOk := oldMap[defaultEndpointName]
	if !oldOk {
		defaultBinding = network.DefaultSpaceId
	}
	if newDefaultBinding, newOk := newMap[defaultEndpointName]; newOk {
		// new default binding supersedes the old default binding
		defaultBinding = newDefaultBinding
	}

	// defaultsMap contains all endpoints that must be bound for the given charm
	// metadata, but we need to figure out which value to use for each key.
	updated := make(map[string]string)
	updated[defaultEndpointName] = defaultBinding
	for key, defaultValue := range defaultsMap {
		effectiveValue := defaultValue

		oldValue, hasOld := oldMap[key]
		if hasOld {
			if oldValue != effectiveValue {
				effectiveValue = oldValue
			}
		} else {
			// Old didn't talk about this value, but maybe we have a default
			effectiveValue = defaultBinding
		}

		newValue, hasNew := newMap[key]
		if hasNew && newValue != effectiveValue {
			effectiveValue = newValue
		}

		updated[key] = effectiveValue
	}

	// Any other bindings in newMap are most likely extraneous, but add them
	// anyway and let the validation handle them.
	for key, newValue := range newMap {
		if _, defaultExists := defaultsMap[key]; !defaultExists {
			updated[key] = newValue
		}
	}
	isModified := false
	if len(updated) != len(oldMap) {
		isModified = true
	} else {
		// If the len() is identical, then we know as long as we iterate all entries, then there is no way to
		// miss an entry. Either they have identical keys and we check all the values, or there is an identical
		// number of new keys and missing keys and we'll notice a missing key.
		for key, val := range updated {
			if oldVal, existed := oldMap[key]; !existed || oldVal != val {
				isModified = true
				break
			}
		}
	}
	logger.Debugf("merged endpoint bindings modified: %t, default: %v, old: %v, new: %v, result: %v",
		isModified, defaultsMap, oldMap, newMap, updated)
	return updated, isModified, nil
}

// createEndpointBindingsOp returns the op needed to create new endpoint
// bindings using the optional givenMap and the specified charm metadata to for
// determining defaults and to validate the effective bindings.
func createEndpointBindingsOp(st *State, key string, givenMap map[string]string, meta *charm.Meta) (txn.Op, error) {
	endpointSpaceIDMap, err := ensureEndpointSpaceID(st, givenMap)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	// No existing map to merge, just use the defaults.
	initialMap, _, err := mergeBindings(endpointSpaceIDMap, nil, meta)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	// Validate the bindings before inserting.
	if err := validateEndpointBindingsForCharm(st, initialMap, meta); err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	return txn.Op{
		C:      endpointBindingsC,
		Id:     key,
		Assert: txn.DocMissing,
		Insert: endpointBindingsDoc{
			Bindings: initialMap,
		},
	}, nil
}

func ensureEndpointSpaceID(st *State, givenMap map[string]string) (map[string]string, error) {
	spacesNamesToIDs, err := st.SpaceIDsByName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	spacesIDsToNames, err := st.SpaceNamesByID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	newMap := make(map[string]string, len(givenMap))
	for endpoint, space := range givenMap {
		if id, haveName := spacesNamesToIDs[space]; haveName {
			newMap[endpoint] = id
			continue
		}
		_, haveID := spacesIDsToNames[space]
		if haveID || space == "" {
			newMap[endpoint] = space
			continue
		}
		return nil, errors.NotFoundf("endpoint %q value %q, space name or ID", endpoint, space)
	}
	return newMap, nil
}

// updateEndpointBindingsOps returns an op list that merges the existing
// bindings with givenMap, using newMeta to validate the merged bindings, and
// asserting that the following items have not changed since we last fetched
// them:
// - names of spaces assigned to endpoints.
// - application unit count.
// - endpoint bindings that we are currently trying to update.
func updateEndpointBindingsOps(st *State, a *Application, givenMap map[string]string, newMeta *charm.Meta) ([]txn.Op, error) {
	var ops []txn.Op

	// Fetch existing bindings.
	oldMap, txnRevno, err := readEndpointBindings(st, a.globalKey())
	if err != nil && !errors.IsNotFound(err) {
		return ops, errors.Trace(err)
	}

	newMap, err := ensureEndpointSpaceID(st, givenMap)
	if err != nil {
		return ops, errors.Trace(err)
	}

	// Merge existing with given as needed.
	updatedMap, isModified, err := mergeBindings(newMap, oldMap, newMeta)
	if err != nil {
		return ops, errors.Trace(err)
	}

	if !isModified {
		return ops, jujutxn.ErrNoOperations
	}

	// Validate the bindings before updating.
	if err := validateEndpointBindingsForCharm(st, updatedMap, newMeta); err != nil {
		return ops, errors.Trace(err)
	}

	// Make sure that all machines which run units of this application
	// contain addresses in the spaces we are trying to bind to.
	if err := validateEndpointBindingsForMachines(st, a, updatedMap); err != nil {
		return ops, errors.Trace(err)
	}

	// Ensure that the spaceIDs needed for the bindings exist.
	for _, spID := range updatedMap {
		sp, err := st.SpaceByID(spID)
		if err != nil {
			return ops, errors.Trace(err)
		}
		ops = append(ops, txn.Op{
			C:      spacesC,
			Id:     sp.doc.DocId,
			Assert: txn.DocExists,
		})
	}

	// To avoid a potential race where units may suddenly appear on a new
	// machine that does not have addresses for all the required spaces
	// while we are applying the txn, we define an assertion on the unit
	// count for the current application.
	ops = append(ops, txn.Op{
		C:      applicationsC,
		Id:     a.doc.DocID,
		Assert: bson.D{{"unitcount", a.UnitCount()}},
	})

	// Prepare the update operations.
	escaped := make(bson.M, len(updatedMap))
	for endpoint, space := range updatedMap {
		escaped[utils.EscapeKey(endpoint)] = space
	}

	updateOp := txn.Op{
		C:      endpointBindingsC,
		Id:     a.globalKey(),
		Update: bson.M{"$set": bson.M{"bindings": escaped}},
	}
	if oldMap != nil {
		// Only assert existing haven't changed when they actually exist.
		updateOp.Assert = bson.D{{"txn-revno", txnRevno}}
	}

	return append(ops, updateOp), nil
}

// validateEndpointBindingsForMachines checks whether the required space
// assignments are actually feasible given the network configuration settings
// of the machines where application units are already running.
func validateEndpointBindingsForMachines(st *State, a *Application, newBindings map[string]string) error {
	// Get a list of deployed machines and create a map where we track the
	// count of deployed machines for each space.
	machineCountInSpace := make(map[string]int)
	deployedMachines, err := a.DeployedMachines()
	if err != nil {
		return err
	}

	for _, m := range deployedMachines {
		machineSpaces, err := m.AllSpaces()
		if err != nil {
			return errors.Annotatef(err, "unable to get space assignments for machine %q", m.Id())
		}
		for spID := range machineSpaces {
			machineCountInSpace[spID]++
		}
	}

	if newDefaultSpace, defined := newBindings[defaultEndpointName]; defined && newDefaultSpace != network.DefaultSpaceId {
		if machineCountInSpace[newDefaultSpace] != len(deployedMachines) {
			msg := "changing default space to %q is not feasible: one or more deployed machines lack an address in this space"
			return st.spaceNotFeasibleError(msg, newDefaultSpace)
		}
	}

	for epName, spID := range newBindings {
		if epName == "" {
			continue
		}
		// TODO(achilleasa): this check is a temporary workaround
		// to allow upgrading charms that define new endpoints
		// which we automatically bind to the default space if
		// the operator does not explicitly try to bind them
		// to a space.
		//
		// If we deploy a charm with a "spaces=xxx" constraint,
		// it will not have a provider address in the default
		// space so the machine-count check below would
		// otherwise fail.
		if spID == network.DefaultSpaceId {
			continue
		}

		// Ensure that all currently deployed machines have an address
		// in the requested space for this binding
		if machineCountInSpace[spID] != len(deployedMachines) {
			msg := fmt.Sprintf("binding endpoint %q to ", epName)
			return st.spaceNotFeasibleError(msg+"space %q is not feasible: one or more deployed machines lack an address in this space", spID)
		}
	}

	return nil
}

func (st *State) spaceNotFeasibleError(msg, id string) error {
	space, err := st.SpaceByID(id)
	if err != nil {
		logger.Errorf(msg, id)
		return errors.Annotatef(err, "cannot get space name for id %q", id)
	}
	return errors.Errorf(msg, space.Name())
}

// removeEndpointBindingsOp returns an op removing the bindings for the given
// key, without asserting they exist in the first place.
func removeEndpointBindingsOp(key string) txn.Op {
	return txn.Op{
		C:      endpointBindingsC,
		Id:     key,
		Remove: true,
	}
}

// readEndpointBindings returns the stored bindings and TxnRevno for the given
// application global key, or an error satisfying errors.IsNotFound() otherwise.
func readEndpointBindings(st *State, key string) (map[string]string, int64, error) {
	doc, err := readEndpointBindingsDoc(st, key)
	if err != nil {
		return nil, 0, err
	}
	return doc.Bindings, doc.TxnRevno, nil
}

// readEndpointBindingsDoc returns the endpoint bindings document for the
// specified key.
func readEndpointBindingsDoc(st *State, key string) (*endpointBindingsDoc, error) {
	endpointBindings, closer := st.db().GetCollection(endpointBindingsC)
	defer closer()

	var doc endpointBindingsDoc
	err := endpointBindings.FindId(key).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("endpoint bindings for %q", key)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get endpoint bindings for %q", key)
	}

	return &doc, nil
}

// validateEndpointBindingsForCharm verifies that all endpoint names in bindings
// are valid for the given charm metadata, and each endpoint is bound to a known
// space - otherwise an error satisfying errors.IsNotValid() will be returned.
func validateEndpointBindingsForCharm(st *State, bindings map[string]string, charmMeta *charm.Meta) error {
	if st == nil {
		return errors.NotValidf("nil state")
	}
	if bindings == nil {
		return errors.NotValidf("nil bindings")
	}
	if charmMeta == nil {
		return errors.NotValidf("nil charm metadata")
	}

	// We do not need the space names, but a handy way to
	// determine valid space IDs.
	spaceIDs, err := st.SpaceNamesByID()
	if err != nil {
		return errors.Trace(err)
	}

	allBindings := DefaultEndpointBindingsForCharm(charmMeta)
	endpointsNamesSet := set.NewStrings()
	for name := range allBindings {
		endpointsNamesSet.Add(name)
	}

	// Ensure there are no unknown endpoints and/or spaces specified.
	//
	// TODO(dimitern): This assumes spaces cannot be deleted when they are used
	// in bindings. In follow-up, this will be enforced by using refcounts on
	// spaces.
	for endpoint, space := range bindings {
		if endpoint != defaultEndpointName && !endpointsNamesSet.Contains(endpoint) {
			return errors.NotValidf("unknown endpoint %q", endpoint)
		}
		if _, ok := spaceIDs[space]; !ok {
			return errors.NotValidf("unknown space %q", space)
		}
	}
	return nil
}

// DefaultEndpointBindingsForCharm populates a bindings map containing each
// endpoint of the given charm metadata (relation name or extra-binding name)
// bound to an empty space.
func DefaultEndpointBindingsForCharm(charmMeta *charm.Meta) map[string]string {
	allRelations := charmMeta.CombinedRelations()
	bindings := make(map[string]string, len(allRelations)+len(charmMeta.ExtraBindings))
	for name := range allRelations {
		bindings[name] = network.DefaultSpaceId
	}
	for name := range charmMeta.ExtraBindings {
		bindings[name] = network.DefaultSpaceId
	}
	return bindings
}

// translateSpaceNameToID takes a map of endpoint bindings to space names and
// and returns a map of endpoint bindings to space ids
func (st *State) translateSpaceNameToID(current map[string]string) (map[string]string, error) {
	retVal := make(map[string]string, len(current))
	namesToIDs, err := st.SpaceIDsByName()
	if err != nil {
		return nil, err
	}
	for k, v := range current {
		if v == network.DefaultSpaceName || v == network.DefaultSpaceId {
			retVal[k] = network.DefaultSpaceId
			continue
		}
		if _, err := st.SpaceByID(v); err == nil {
			// If one binding endpoint is a SpaceID, so are the rest.
			return current, nil
		}
		spaceID, found := namesToIDs[v]
		if !found {
			return nil, errors.NotFoundf("space id for space %q", v)
		}
		retVal[k] = spaceID
	}
	return retVal, nil
}
