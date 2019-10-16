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

// Merge the default bindings based on the given charm metadata, overriding
// them first with matching current values, and then with mergeWith values
// (for the same keys). Current values and mergeWith are both optional and
// will ignored when empty. The current object contains the combined finalized
// bindings. Returns true/false if there are any actual differences.
func (b *Bindings) Merge(mergeWith map[string]string, meta *charm.Meta) (bool, error) {
	defaultsMap := DefaultEndpointBindingsForCharm(meta)
	defaultBinding, oldOk := mergeWith[defaultEndpointName]
	if !oldOk {
		defaultBinding = network.DefaultSpaceId
	}
	if newDefaultBinding, newOk := b.bindingsMap[defaultEndpointName]; newOk {
		// new default binding supersedes the old default binding
		defaultBinding = newDefaultBinding
	}

	// defaultsMap contains all endpoints that must be bound for the given charm
	// metadata, but we need to figure out which value to use for each key.
	updated := make(map[string]string)
	updated[defaultEndpointName] = defaultBinding
	for key, defaultValue := range defaultsMap {
		effectiveValue := defaultValue

		oldValue, hasOld := mergeWith[key]
		if hasOld {
			if oldValue != effectiveValue {
				effectiveValue = oldValue
			}
		} else {
			// Old didn't talk about this value, but maybe we have a default
			effectiveValue = defaultBinding
		}

		newValue, hasNew := b.bindingsMap[key]
		if hasNew && newValue != effectiveValue {
			effectiveValue = newValue
		}

		updated[key] = effectiveValue
	}

	// Any other bindings in newMap are most likely extraneous, but add them
	// anyway and let the validation handle them.
	for key, newValue := range b.bindingsMap {
		if _, defaultExists := defaultsMap[key]; !defaultExists {
			updated[key] = newValue
		}
	}
	isModified := false
	if len(updated) != len(mergeWith) {
		isModified = true
	} else {
		// If the len() is identical, then we know as long as we iterate all entries, then there is no way to
		// miss an entry. Either they have identical keys and we check all the values, or there is an identical
		// number of new keys and missing keys and we'll notice a missing key.
		for key, val := range updated {
			if oldVal, existed := mergeWith[key]; !existed || oldVal != val {
				isModified = true
				break
			}
		}
	}
	logger.Debugf("merged endpoint bindings modified: %t, default: %v, old: %v, new: %v, result: %v",
		isModified, defaultsMap, mergeWith, b.bindingsMap, updated)
	if isModified {
		b.bindingsMap = updated
	}
	return isModified, nil
}

// createOp returns the op needed to create new endpoint bindings using the
// optional current bindings and the specified charm metadata to for
// determining defaults and to validate the effective bindings.
func (b *Bindings) createOp(meta *charm.Meta) (txn.Op, error) {
	if b.app == nil {
		return txn.Op{}, errors.Trace(errors.New("programming error: app is a nil pointer"))
	}
	// No existing map to Merge, just use the defaults.
	_, err := b.Merge(nil, meta)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	// Validate the bindings before inserting.
	if err := b.validateForCharm(meta); err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	return txn.Op{
		C:      endpointBindingsC,
		Id:     b.app.globalKey(),
		Assert: txn.DocMissing,
		Insert: endpointBindingsDoc{
			Bindings: b.Map(),
		},
	}, nil
}

// updateOps returns an op list that merges the current bindings and the old
// bindings, using newMeta to validate the merged bindings, and asserting that
// the following items have not changed since we last fetched them:
// - ids of spaces assigned to endpoints.
// - application unit count.
// - endpoint bindings that we are currently trying to update.
func (b *Bindings) updateOps(newMeta *charm.Meta) ([]txn.Op, error) {
	var ops []txn.Op

	if b.app == nil {
		return ops, errors.Trace(errors.New("programming error: app is a nil pointer"))
	}

	// Fetch existing bindings.
	oldMap, txnRevno, err := readEndpointBindings(b.app.st, b.app.globalKey())
	if err != nil && !errors.IsNotFound(err) {
		return ops, errors.Trace(err)
	}

	// Merge existing with given as needed.
	isModified, err := b.Merge(oldMap, newMeta)
	if err != nil {
		return ops, errors.Trace(err)
	}

	if !isModified {
		return ops, jujutxn.ErrNoOperations
	}

	// Validate the bindings before updating.
	if err := b.validateForCharm(newMeta); err != nil {
		return ops, errors.Trace(err)
	}

	// Make sure that all machines which run units of this application
	// contain addresses in the spaces we are trying to bind to.
	if err := b.validateForMachines(); err != nil {
		return ops, errors.Trace(err)
	}

	// Ensure that the spaceIDs needed for the bindings exist.
	for _, spID := range b.Map() {
		sp, err := b.st.SpaceByID(spID)
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
		Id:     b.app.doc.DocID,
		Assert: bson.D{{"unitcount", b.app.UnitCount()}},
	})

	// Prepare the update operations.
	escaped := make(bson.M, len(b.Map()))
	for endpoint, space := range b.Map() {
		escaped[utils.EscapeKey(endpoint)] = space
	}

	updateOp := txn.Op{
		C:      endpointBindingsC,
		Id:     b.app.globalKey(),
		Update: bson.M{"$set": bson.M{"bindings": escaped}},
	}
	if oldMap != nil {
		// Only assert existing haven't changed when they actually exist.
		updateOp.Assert = bson.D{{"txn-revno", txnRevno}}
	}

	return append(ops, updateOp), nil
}

// validateForMachines checks whether the required space
// assignments are actually feasible given the network configuration settings
// of the machines where application units are already running.
func (b *Bindings) validateForMachines() error {
	if b.app == nil {
		return errors.Trace(errors.New("programming error: app is a nil pointer"))
	}
	// Get a list of deployed machines and create a map where we track the
	// count of deployed machines for each space.
	machineCountInSpace := make(map[string]int)
	deployedMachines, err := b.app.DeployedMachines()
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

	if newDefaultSpace, defined := b.bindingsMap[defaultEndpointName]; defined && newDefaultSpace != network.DefaultSpaceId {
		if machineCountInSpace[newDefaultSpace] != len(deployedMachines) {
			msg := "changing default space to %q is not feasible: one or more deployed machines lack an address in this space"
			return b.spaceNotFeasibleError(msg, newDefaultSpace)
		}
	}

	for epName, spID := range b.bindingsMap {
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
			return b.spaceNotFeasibleError(msg+"space %q is not feasible: one or more deployed machines lack an address in this space", spID)
		}
	}

	return nil
}

func (b *Bindings) spaceNotFeasibleError(msg, id string) error {
	space, err := b.st.SpaceByID(id)
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

// validateForCharm verifies that all endpoint names in bindings
// are valid for the given charm metadata, and each endpoint is bound to a known
// space - otherwise an error satisfying errors.IsNotValid() will be returned.
func (b *Bindings) validateForCharm(charmMeta *charm.Meta) error {
	if b.bindingsMap == nil {
		return errors.NotValidf("nil bindings")
	}
	if charmMeta == nil {
		return errors.NotValidf("nil charm metadata")
	}

	// We do not need the space names, but a handy way to
	// determine valid space IDs.
	spaceIDs, err := b.st.SpaceNamesByID()
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
	for endpoint, space := range b.bindingsMap {
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

//go:generate mockgen -package mocks -destination mocks/endpointbinding_mock.go github.com/juju/juju/state EndpointBinding
type EndpointBinding interface {
	SpaceByID(id string) (*Space, error)
	SpaceIDsByName() (map[string]string, error)
	SpaceNamesByID() (map[string]string, error)
}

type Bindings struct {
	st  EndpointBinding
	app *Application
	bindingsMap
}

// NewBindings returns a bindings guaranteed to be in space id format.
func NewBindings(st EndpointBinding, givenMap map[string]string) (*Bindings, error) {
	spacesNamesToIDs, err := st.SpaceIDsByName()
	if err != nil {
		return nil, errors.Trace(err)
	}
	haveAllNames := allOfOne(spacesNamesToIDs, givenMap)

	spacesIDsToNames, err := st.SpaceNamesByID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	haveAllIDs := allOfOne(spacesIDsToNames, givenMap)

	// Ensure the spaces values are all names OR ids in the
	// given map.
	var newMap map[string]string
	switch {
	case !haveAllIDs && !haveAllNames:
		return nil, errors.NotValidf("%v contains both space names and ids", givenMap)
	case haveAllNames && haveAllIDs:
		// The givenMap is empty, just continue, the loop will be skipped.
		newMap = make(map[string]string, len(givenMap))
	case haveAllNames:
		logger.Criticalf("allNames %+v", givenMap)
		logger.Criticalf("%+v", spacesNamesToIDs)
		newMap, err = newBindingsFromNames(spacesNamesToIDs, givenMap)
	case haveAllIDs:
		logger.Criticalf("allIDs %+v", givenMap)
		logger.Criticalf("%+v", spacesIDsToNames)
		newMap, err = newBindingsFromIDs(spacesIDsToNames, givenMap)
	}

	return &Bindings{st: st, bindingsMap: newMap}, err
}

func allOfOne(currentSpaces, givenMap map[string]string) bool {
	for _, id := range givenMap {
		if _, ok := currentSpaces[id]; !ok && id != "" {
			return false
		}
	}
	return true
}

func newBindingsFromNames(verificationMap, givenMap map[string]string) (map[string]string, error) {
	newMap := make(map[string]string, len(givenMap))
	for epName, name := range givenMap {
		if name == "" {
			newMap[epName] = ""
			continue
		}
		// check that the name is valid and get id.
		value, ok := verificationMap[name]
		if !ok {
			return nil, errors.NotFoundf("epName %q space name value %q", epName, name)
		}
		newMap[epName] = value
	}
	return newMap, nil
}

func newBindingsFromIDs(verificationMap, givenMap map[string]string) (map[string]string, error) {
	newMap := make(map[string]string, len(givenMap))
	for epName, id := range givenMap {
		if id == "" {
			newMap[epName] = ""
			continue
		}
		// check that the id is valid.
		if _, ok := verificationMap[id]; ok || id == "" {
			newMap[epName] = id
			continue
		}
		return nil, errors.NotFoundf("epName %q space id value %q", epName, id)
	}
	return newMap, nil
}

// MapWithSpaceNames returns the current bindingMap with space names rather than ids.
func (b *Bindings) MapWithSpaceNames() (map[string]string, error) {
	retVal := make(map[string]string, len(b.bindingsMap))
	namesToIDs, err := b.st.SpaceIDsByName()
	if err != nil {
		return nil, err
	}
	for k, v := range b.bindingsMap {
		if v == network.DefaultSpaceName || v == network.DefaultSpaceId {
			retVal[k] = network.DefaultSpaceId
			continue
		}
		if _, err := b.st.SpaceByID(v); err == nil {
			// If one binding endpoint is a SpaceID, so are the rest.
			return b.bindingsMap, nil
		}
		spaceID, found := namesToIDs[v]
		if !found {
			return nil, errors.NotFoundf("space id for space %q", v)
		}
		retVal[k] = spaceID
	}
	return retVal, nil
}

// Map returns the current bindingMap.
func (b *Bindings) Map() map[string]string {
	return b.bindingsMap
}
