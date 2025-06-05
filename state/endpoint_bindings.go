// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/mongo/utils"
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

// Merge the default bindings based on the given charm metadata with the
// current bindings, overriding with mergeWith values (for the same keys).
// Current values and mergeWith are both optional and will ignored when
// empty. The current object contains the combined finalized bindings.
// Returns true/false if there are any actual differences.
func (b *Bindings) Merge(mergeWith map[string]string, meta *charm.Meta) (bool, error) {
	// Verify the bindings to be merged, and ensure we're merging with
	// space ids.
	merge, err := NewBindings(b.st, mergeWith)
	if err != nil {
		return false, errors.Trace(err)
	}
	mergeMap := merge.Map()

	defaultsMap, err := DefaultEndpointBindingsForCharm(b.st, meta)
	if err != nil {
		return false, errors.Trace(err)
	}

	defaultBinding, mergeOK := b.bindingsMap[defaultEndpointName]
	if !mergeOK {
		// TODO (manadart 2024-01-29): The alpha space ID here is scaffolding and
		// should be replaced with the configured model default space upon
		// migrating this logic to Dqlite.
		defaultBinding = network.AlphaSpaceId.String()
	}
	if newDefaultBinding, newOk := mergeMap[defaultEndpointName]; newOk {
		// new default binding supersedes the old default binding
		defaultBinding = newDefaultBinding
	}

	// defaultsMap contains all endpoints that must be bound for the given charm
	// metadata, but we need to figure out which value to use for each key.
	updated := make(map[string]string)
	updated[defaultEndpointName] = defaultBinding
	for key, defaultValue := range defaultsMap {
		effectiveValue := defaultValue

		currentValue, hasCurrent := b.bindingsMap[key]
		if hasCurrent {
			if currentValue != effectiveValue {
				effectiveValue = currentValue
			}
		} else {
			// current didn't talk about this value, but maybe we have a default
			effectiveValue = defaultBinding
		}

		mergeValue, hasMerge := mergeMap[key]
		if hasMerge && mergeValue != effectiveValue && mergeValue != "" {
			effectiveValue = mergeValue
		}

		updated[key] = effectiveValue
	}

	// Any other bindings in mergeWith Map are most likely extraneous, but add them
	// anyway and let the validation handle them.
	for key, newValue := range mergeMap {
		if _, defaultExists := defaultsMap[key]; !defaultExists {
			updated[key] = newValue
		}
	}
	isModified := false
	if len(updated) != len(b.bindingsMap) {
		isModified = true
	} else {
		// If the len() is identical, then we know as long as we iterate all entries, then there is no way to
		// miss an entry. Either they have identical keys and we check all the values, or there is an identical
		// number of new keys and missing keys and we'll notice a missing key.
		for key, val := range updated {
			if oldVal, existed := b.bindingsMap[key]; !existed || oldVal != val {
				isModified = true
				break
			}
		}
	}
	logger.Debugf(context.TODO(), "merged endpoint bindings modified: %t, default: %v, current: %v, mergeWith: %v, after: %v",
		isModified, defaultsMap, b.bindingsMap, mergeMap, updated)
	if isModified {
		b.bindingsMap = updated
	}
	return isModified, nil
}

// createOp returns the op needed to create new endpoint bindings using the
// optional current bindings and the specified charm metadata to for
// determining defaults and to validate the effective bindings.
func (b *Bindings) createOp(bindings map[string]string, meta *charm.Meta) (txn.Op, error) {
	if b.app == nil {
		return txn.Op{}, errors.Trace(errors.New("programming error: app is a nil pointer"))
	}
	// No existing map to Merge, just use the defaults.
	_, err := b.Merge(bindings, meta)
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

// updateOps returns an op list to update the endpoint bindings for an application.
// The implementation calculates the final set of bindings by merging the provided
// newMap into the existing set of bindings. The final bindings are validated
// in two ways:
//  1. we make sure that the endpoint names in the binding map are all present
//     in the provided charm metadata and that the space IDs actually exist.
//  2. we check that all existing units for the application are executing on
//     machines that have an address in each space we are binding to. This
//     check can be circumvented by setting the force argument to true.
//
// The returned operation list includes additional operations that perform
// the following assertions:
//   - assert that the unit count has not changed while the txn is in progress.
//   - assert that the spaces we are binding to have not been deleted.
//   - assert that the existing bindings we used for calculating the merged set
//     of bindings have not changed while the txn is in progress.
func (b *Bindings) updateOps(txnRevno int64, newMap map[string]string, newMeta *charm.Meta, force bool) ([]txn.Op, error) {
	var ops []txn.Op

	if b.app == nil {
		return ops, errors.Trace(errors.New("programming error: app is a nil pointer"))
	}

	useTxnRevno := len(b.bindingsMap) > 0

	// Merge existing with new as needed.
	isModified, err := b.Merge(newMap, newMeta)
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
	if !force {
		// TODO(nvinuesa): This check must be reimplemented once we
		// migrate endpoint bindings to dqlite and check the spaces
		// a machine has addresses on.
		// Look for the `validateForMachines()` method on 3.x branch(es).
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

	_, bindingsErr := readEndpointBindingsDoc(b.app.st, b.app.globalKey())
	if bindingsErr != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	if err != nil {
		// No bindings to update.
		return ops, nil
	}
	updateOp := txn.Op{
		C:      endpointBindingsC,
		Id:     b.app.globalKey(),
		Assert: txn.DocExists,
		Update: bson.M{"$set": bson.M{"bindings": escaped}},
	}
	if useTxnRevno {
		// Only assert existing haven't changed when they actually exist.
		updateOp.Assert = bson.D{{"txn-revno", txnRevno}}
	}

	return append(ops, updateOp), nil
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

	allBindings, err := DefaultEndpointBindingsForCharm(b.st, charmMeta)
	if err != nil {
		return errors.Trace(err)
	}
	endpointsNamesSet := set.NewStrings()
	for name := range allBindings {
		endpointsNamesSet.Add(name)
	}

	// Ensure there are no unknown endpoints and/or spaces specified.
	// This assumes spaces cannot be deleted when they are used
	// in bindings.
	for endpoint := range b.bindingsMap {
		if endpoint != defaultEndpointName && !endpointsNamesSet.Contains(endpoint) {
			return errors.NotValidf("unknown endpoint %q", endpoint)
		}
		// TODO (manadart (2024-03-25): When cutting this over to Dqlite,
		// ensure that we validate that the space IDs are valid in the model.
	}
	return nil
}

// DefaultEndpointBindingsForCharm populates a bindings map containing each
// endpoint of the given charm metadata (relation name or extra-binding name)
// bound to an empty space.
// TODO (manadart 2024-01-29): The alpha space ID here is scaffolding and
// should be replaced with the configured model default space upon
// migrating this logic to Dqlite.
func DefaultEndpointBindingsForCharm(_ any, charmMeta *charm.Meta) (map[string]string, error) {
	allRelations := charmMeta.CombinedRelations()
	bindings := make(map[string]string, len(allRelations)+len(charmMeta.ExtraBindings))
	for name := range allRelations {
		bindings[name] = network.AlphaSpaceId.String()
	}
	for name := range charmMeta.ExtraBindings {
		bindings[name] = network.AlphaSpaceId.String()
	}
	return bindings, nil
}

// Bindings are EndpointBindings.
type Bindings struct {
	st  any
	app *Application
	bindingsMap
}

// NewBindings returns a bindings guaranteed to be in space id format.
func NewBindings(st any, givenMap map[string]string) (*Bindings, error) {
	newMap := make(map[string]string, len(givenMap))
	for epName, id := range givenMap {
		if id == "" {
			return nil, errors.NotValidf("bindings map with empty space ID")
		}

		// TODO (manadart (2024-03-25): When cutting this over to Dqlite,
		// ensure that we validate that the space IDs are valid in the model.
		newMap[epName] = id
	}

	return &Bindings{st: st, bindingsMap: newMap}, nil
}

// MapWithSpaceNames returns the current bindingMap with space names rather than ids.
func (b *Bindings) MapWithSpaceNames(lookup network.SpaceInfos) (map[string]string, error) {
	// Handle the fact that space name lookup can be nil or empty.
	if lookup == nil || (len(b.bindingsMap) > 0 && len(lookup) == 0) {
		return nil, errors.NotValidf("empty space lookup")
	}

	retVal := make(map[string]string, len(b.bindingsMap))

	// Assume that b.bindings is always in space id format due to
	// Bindings constructor.
	for k, v := range b.bindingsMap {
		spaceInfo := lookup.GetByID(network.SpaceUUID(v))
		if spaceInfo == nil {
			return nil, errors.NotFoundf("space with ID %q", v)
		}
		retVal[k] = string(spaceInfo.Name)
	}
	return retVal, nil
}

// Map returns the current bindingMap with space ids.
func (b *Bindings) Map() map[string]string {
	return b.bindingsMap
}
