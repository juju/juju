// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/charm/v7"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
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
		var err error
		defaultBinding, err = b.st.DefaultEndpointBindingSpace()
		if err != nil {
			return false, errors.Trace(err)
		}
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
	logger.Debugf("merged endpoint bindings modified: %t, default: %v, current: %v, mergeWith: %v, after: %v",
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
// 1) we make sure that the endpoint names in the binding map are all present
//    in the provided charm metadata and that the space IDs actually exist.
// 2) we check that all existing units for the application are executing on
//    machines that have an address in each space we are binding to. This
//    check can be circumvented by setting the force argument to true.
//
// The returned operation list includes additional operations that perform
// the following assertions:
// - assert that the unit count has not changed while the txn is in progress.
// - assert that the spaces we are binding to have not been deleted.
// - assert that the existing bindings we used for calculating the merged set
//   of bindings have not changed while the txn is in progress.
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
		if err := b.validateForMachines(); err != nil {
			return ops, errors.Trace(err)
		}
	}

	// Ensure that the spaceIDs needed for the bindings exist.
	for _, spID := range b.Map() {
		sp, err := b.st.Space(spID)
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
	if useTxnRevno {
		// Only assert existing haven't changed when they actually exist.
		updateOp.Assert = bson.D{{"txn-revno", txnRevno}}
	}

	return append(ops, updateOp), nil
}

// validateForMachines ensures that the current set of endpoint to space ID
// bindings (including the default space ID for the app) are feasible given the
// the network configuration settings of the machines where application units
// are already running.
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

	// We only need to validate changes to the default space ID for the
	// application if the operator is trying to change it to something
	// other than network.DefaultSpaceID
	if newDefaultSpaceIDForApp, defined := b.bindingsMap[defaultEndpointName]; defined && newDefaultSpaceIDForApp != network.AlphaSpaceId {
		if machineCountInSpace[newDefaultSpaceIDForApp] != len(deployedMachines) {
			msg := "changing default space to %q is not feasible: one or more deployed machines lack an address in this space"
			return b.spaceNotFeasibleError(msg, newDefaultSpaceIDForApp)
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
		if spID == network.AlphaSpaceId {
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
	space, err := b.st.Space(id)
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

	spaceInfos, err := b.st.AllSpaceInfos()
	if err != nil {
		return errors.Trace(err)
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
	//
	// TODO(dimitern): This assumes spaces cannot be deleted when they are used
	// in bindings. In follow-up, this will be enforced by using refcounts on
	// spaces.
	for endpoint, space := range b.bindingsMap {
		if endpoint != defaultEndpointName && !endpointsNamesSet.Contains(endpoint) {
			return errors.NotValidf("unknown endpoint %q", endpoint)
		}
		if !spaceInfos.ContainsID(space) {
			return errors.NotValidf("unknown space %q", space)
		}
	}
	return nil
}

// DefaultEndpointBindingSpace returns the current space ID to be used for
// the default endpoint binding.
func (st *State) DefaultEndpointBindingSpace() (string, error) {
	model, err := st.Model()
	if err != nil {
		return "", errors.Trace(err)
	}

	cfg, err := model.Config()
	if err != nil {
		return "", errors.Trace(err)
	}

	defaultBinding := network.AlphaSpaceId

	space, err := st.SpaceByName(cfg.DefaultSpace())
	if err != nil && !errors.IsNotFound(err) {
		return "", errors.Trace(err)
	}
	if err == nil {
		defaultBinding = space.Id()
	}

	return defaultBinding, nil
}

// DefaultEndpointBindingsForCharm populates a bindings map containing each
// endpoint of the given charm metadata (relation name or extra-binding name)
// bound to an empty space.
func DefaultEndpointBindingsForCharm(st EndpointBinding, charmMeta *charm.Meta) (map[string]string, error) {
	defaultBindingSpaceID, err := st.DefaultEndpointBindingSpace()
	if err != nil {
		return nil, err
	}
	allRelations := charmMeta.CombinedRelations()
	bindings := make(map[string]string, len(allRelations)+len(charmMeta.ExtraBindings))
	for name := range allRelations {
		bindings[name] = defaultBindingSpaceID
	}
	for name := range charmMeta.ExtraBindings {
		bindings[name] = defaultBindingSpaceID
	}
	return bindings, nil
}

// EndpointBinding are the methods necessary for exported methods of
// Bindings to work.
//
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/endpointbinding_mock.go github.com/juju/juju/state EndpointBinding
type EndpointBinding interface {
	network.SpaceLookup
	DefaultEndpointBindingSpace() (string, error)
	Space(id string) (*Space, error)
}

// Bindings are EndpointBindings.
type Bindings struct {
	st  EndpointBinding
	app *Application
	bindingsMap
}

// NewBindings returns a bindings guaranteed to be in space id format.
func NewBindings(st EndpointBinding, givenMap map[string]string) (*Bindings, error) {
	// namesErr and idError are only problems if both are not nil.
	spaceInfos, err := st.AllSpaceInfos()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// If givenMap contains space names empty values are allowed (e.g. they
	// may be present when migrating a model from a 2.6.x controller).
	namesErr := allOfOne(spaceInfos.ContainsName, givenMap, true)

	// If givenMap contains empty values then the map most probably contains
	// space names. Therefore, we want allOfOne to be strict and bail out
	// if it sees any empty values.
	idErr := allOfOne(spaceInfos.ContainsID, givenMap, false)

	// Ensure the spaces values are all names OR ids in the
	// given map.
	var newMap map[string]string
	switch {
	case namesErr == nil && idErr == nil:
		// The givenMap is empty or has empty endpoints
		if len(givenMap) > 0 {
			newMap = givenMap
			break
		}
		newMap = make(map[string]string, len(givenMap))

	case namesErr == nil && idErr != nil:
		newMap, err = newBindingsFromNames(spaceInfos, givenMap)
	case idErr == nil && namesErr != nil:
		newMap, err = newBindingsFromIDs(spaceInfos, givenMap)
	default:
		logger.Errorf("%s", namesErr)
		logger.Errorf("%s", idErr)
		return nil, errors.NotFoundf("space")
	}

	return &Bindings{st: st, bindingsMap: newMap}, err
}

func allOfOne(foundValue func(string) bool, givenMap map[string]string, allowEmptyValues bool) error {
	for k, v := range givenMap {
		if !foundValue(v) && (v != "" || (v == "" && !allowEmptyValues)) {
			return errors.NotFoundf("endpoint %q, value %q, space name or id", k, v)
		}
	}
	return nil
}

func newBindingsFromNames(spaceInfos network.SpaceInfos, givenMap map[string]string) (map[string]string, error) {
	newMap := make(map[string]string, len(givenMap))
	for epName, name := range givenMap {
		if name == "" {
			newMap[epName] = network.AlphaSpaceId
			continue
		}
		// check that the name is valid and get id.
		info := spaceInfos.GetByName(name)
		if info == nil {
			return nil, errors.NotFoundf("programming error: epName %q space name value %q", epName, name)
		}
		newMap[epName] = info.ID
	}
	return newMap, nil
}

func newBindingsFromIDs(spaceInfos network.SpaceInfos, givenMap map[string]string) (map[string]string, error) {
	newMap := make(map[string]string, len(givenMap))
	for epName, id := range givenMap {
		if id == "" {
			// This is most probably a set of bindings to space names.
			return nil, errors.NotValidf("bindings map with empty space ID")
		}
		// check that the id is valid.
		if !spaceInfos.ContainsID(id) {
			return nil, errors.NotFoundf("programming error: epName %q space id value %q", epName, id)
		}

		newMap[epName] = id
	}
	return newMap, nil
}

// MapWithSpaceNames returns the current bindingMap with space names rather than ids.
func (b *Bindings) MapWithSpaceNames() (map[string]string, error) {
	retVal := make(map[string]string, len(b.bindingsMap))
	lookup, err := b.st.AllSpaceInfos()
	if err != nil {
		return nil, err
	}

	// Assume that b.bindings is always in space id format due to
	// Bindings constructor.
	for k, v := range b.bindingsMap {
		spaceInfo := lookup.GetByID(v)
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
