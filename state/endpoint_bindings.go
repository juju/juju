// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"

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

// Map returns the current bindingMap with space ids.
func (b *Bindings) Map() map[string]string {
	return b.bindingsMap
}
