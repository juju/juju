// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// endpointBindingsDoc represents how a service endpoints are bound to spaces.
// The DocID field contains the service's global key, so there is at most one
// endpointBindingsDoc per service.
type endpointBindingsDoc struct {
	// DocID is always the same as a service's global key.
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`

	// Bindings maps a service endpoint name to the space name it is bound to.
	Bindings bindingsMap `bson:"bindings"`

	// TxnRevno is used to assert the collection have not changed since this
	// document was fetched.
	TxnRevno int64 `bson:"txn-revno"`
}

// bindingsMap is the underlying type stored in mongo for bindings.
type bindingsMap map[string]string

// SetBSON ensures any special characters ($ or .) are unescaped in keys after
// unmarshalling the raw BSON coming from the stored document.
func (bp *bindingsMap) SetBSON(raw bson.Raw) error {
	rawMap := make(map[string]string)
	if err := raw.Unmarshal(rawMap); err != nil {
		return err
	}
	for key, value := range rawMap {
		newKey := unescapeReplacer.Replace(key)
		if newKey != key {
			delete(rawMap, key)
		}
		rawMap[newKey] = value
	}
	*bp = bindingsMap(rawMap)
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
		newKey := escapeReplacer.Replace(key)
		rawMap[newKey] = value
	}
	return rawMap, nil
}

// prepareAddServiceArgsEndpointBindings ensures the given args contain valid
// EndpointBindings map. The validated and possibly changed bindings to store
// are part of the returned txn.Op on success. Returns an error, satisfying
// errors.IsNotValid() for invalid specified bindings, or satisfying
// errors.IsNotSupported() when bindings should not be stored for the service.
func prepareAddServiceArgsEndpointBindingsOp(st *State, args AddServiceArgs) (txn.Op, error) {

	charmMeta := args.Charm.Meta()
	defaultBindings, endpoints, spaces, err := prepareToValidateBindingsForCharm(st, charmMeta)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	givenBindings := args.EndpointBindings
	// No service yet, so no pre-existing bindings to merge or keys to remove.
	mergedWithDefaults, _ := mergeEndpointBindings(givenBindings, nil, defaultBindings)

	if err := validateEndpointBindings(mergedWithDefaults, endpoints, spaces); err != nil {
		return txn.Op{}, errors.Annotate(err, "invalid endpoint bindings provided")
	}

	return txn.Op{
		C:      endpointBindingsC,
		Id:     serviceGlobalKey(args.Name),
		Assert: txn.DocMissing,
		Insert: endpointBindingsDoc{
			Bindings: mergedWithDefaults,
		},
	}, nil
}

// prepareToValidateBindingsForCharm returns the necessary information to
// validate endpoint bindings for the given charmMeta, including the default
// bindings to use as fallback, and two set.Strings - all charm endpoint names,
// and known space names. Returns an error satisfying errors.IsNotSupported()
// when bindings are not supported and should not be created for serivce using
// the given charmMeta.
func prepareToValidateBindingsForCharm(st *State, charmMeta *charm.Meta) (defaults map[string]string, endpointNames, knownSpaceNames set.Strings, err error) {

	controllerSpace, err := ControllerSpaceName(st)
	if err != nil && errors.IsNotFound(err) {
		return nil, nil, nil, errors.NewNotSupported(err, "cannot prepare default endpoint bindings")
	} else if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}

	endpointNames, err = getAllCharmEndpointNames(charmMeta)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}

	knownSpaceNames, err = getAllSpaceNames(st)
	if err != nil {
		return nil, nil, nil, errors.Trace(err)
	}

	defaults = make(map[string]string, len(endpointNames))
	for endpoint := range endpointNames {
		defaults[endpoint] = controllerSpace
	}

	return defaults, endpointNames, knownSpaceNames, nil
}

// mergeEndpointBindings returns the effective bindings map and set of of
// removed endpoint names (if any) by merging newBindings, existingBindings, and
// defaultBindings. No validation is performed any of the arguments or the
// result.
func mergeEndpointBindings(newBindings, existingBindings, defaultBindings map[string]string) (map[string]string, set.Strings) {
	// Use defaultBindings' contents for unspecified bindings as it contains all
	// possible endpoints.
	updated := make(map[string]string)
	for endpoint, defaultValue := range defaultBindings {
		effectiveValue := defaultValue

		oldValue, hasOld := existingBindings[endpoint]
		if hasOld && oldValue != effectiveValue {
			effectiveValue = oldValue
		}

		newValue, hasNew := newBindings[endpoint]
		if hasNew && newValue != effectiveValue {
			effectiveValue = newValue
		}

		updated[endpoint] = effectiveValue
	}

	// Any extra bindings in newBindings are most likely extraneous, but add
	// them anyway and let the validation handle them.
	for endpoint, newValue := range newBindings {
		if _, defaultExists := defaultBindings[endpoint]; !defaultExists {
			updated[endpoint] = newValue
		}
	}

	// All defaults were processed, so anything else in existingBindings not
	// about to be updated and not having a default needs to be removed.
	removedKeys := set.NewStrings()
	for endpoint := range existingBindings {
		if _, updating := updated[endpoint]; !updating {
			removedKeys.Add(endpoint)
		}
		if _, defaultExists := defaultBindings[endpoint]; !defaultExists {
			removedKeys.Add(endpoint)
		}
	}
	return updated, removedKeys
}

// validateBindingsSpaceNames ensures the given bindings map contains only
// non-empty endpoint and space names, part of the given validEndpointNames or
// knownSpaceNames sets, respectively, or an error satisfying
// errors.IsNotValid() otherwise.
func validateEndpointBindings(bindings map[string]string, validEndpointNames, knownSpaceNames set.Strings) error {
	// TODO(dimitern): This assumes spaces cannot be deleted when they are used
	// in bindings, but until we have reference counting for spaces we can't
	// enforce that assumption.
	for endpoint, space := range bindings {
		if endpoint == "" {
			return errors.NotValidf("unbound space %q", space)
		}

		if !validEndpointNames.Contains(endpoint) {
			return errors.NotValidf("unknown endpoint %q", endpoint)
		}

		if space == "" {
			return errors.NotValidf("unbound endpoint %q", endpoint)
		}

		if !knownSpaceNames.Contains(space) {
			return errors.NotValidf("unknown space %q", space)
		}
	}
	return nil
}

// prepareServiceSetCharmEndpointBindingsOp takes care of merging the optional
// newBindings map with both the existing bindings (if any) for the given
// serviceGlobalKey, and with the defaults for newCharmMeta, returning the
// txn.Op to use or txn.ErrNoOperations error if no update is needed. Returns an
// error satisfying errors.IsNotSupported(), if bindings are not supported.
// Returns an error satisfying errors.IsNotValid() when invalid newBindings are
// specified.
func prepareSetCharmEndpointBindingsOp(st *State, serviceGlobalKey string, newBindings map[string]string, newCharmMeta *charm.Meta) (txn.Op, error) {
	existingBindings, txnRevno, err := getEndpointBindings(st, serviceGlobalKey)
	if err != nil && !errors.IsNotFound(err) {
		return txn.Op{}, errors.Trace(err)
	}

	defaultBindings, endpoints, spaces, err := prepareToValidateBindingsForCharm(st, newCharmMeta)
	if err != nil && errors.IsNotSupported(err) {
		logger.Warningf("not updating endpoint bindings for %q: %v", serviceGlobalKey, err)
		return txn.Op{}, jujutxn.ErrNoOperations
	} else if err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	mergedWithDefaults, removedKeys := mergeEndpointBindings(newBindings, existingBindings, defaultBindings)

	if err := validateEndpointBindings(mergedWithDefaults, endpoints, spaces); err != nil {
		return txn.Op{}, errors.Annotate(err, "invalid endpoint bindings provided")
	}

	// Each key we're updating needs to be sanitized for possible special characters.
	sanitize := inSubdocEscapeReplacer("bindings")

	changes := make(bson.M, len(mergedWithDefaults))
	for endpoint, space := range mergedWithDefaults {
		changes[sanitize(endpoint)] = space
	}

	deletes := make(bson.M, len(removedKeys))
	for _, endpoint := range removedKeys.Values() {
		deletes[sanitize(endpoint)] = 1
	}

	var update bson.D
	if len(changes) != 0 {
		update = append(update, bson.DocElem{Name: "$set", Value: changes})
	}
	if len(deletes) != 0 {
		update = append(update, bson.DocElem{Name: "$unset", Value: deletes})
	}
	if len(update) == 0 {
		return txn.Op{}, jujutxn.ErrNoOperations
	}

	return txn.Op{
		C:      endpointBindingsC,
		Id:     serviceGlobalKey,
		Assert: bson.D{{"txn-revno", txnRevno}},
		Update: update,
	}, nil
}

// getEndpointBindings returns the endpoint bindings and TxnRevno for the given
// serviceGlobalKey. When no bindings exist for serviceGlobalKey, an empty
// (non-nil) map and an error satisfying errors.IsNotFound() are returned.
func getEndpointBindings(st *State, serviceGlobalKey string) (map[string]string, int64, error) {
	endpointBindings, closer := st.getCollection(endpointBindingsC)
	defer closer()

	var doc endpointBindingsDoc
	err := endpointBindings.FindId(serviceGlobalKey).One(&doc)

	if err == mgo.ErrNotFound {
		emptyBindings := make(map[string]string)
		return emptyBindings, 0, errors.NotFoundf("endpoint bindings for %q", serviceGlobalKey)
	}
	if err != nil {
		return nil, 0, errors.Annotatef(err, "cannot get endpoint bindings for %q", serviceGlobalKey)
	}

	return doc.Bindings, doc.TxnRevno, nil
}

// removeEndpointBindingsOp returns a txn.Op that removes the bindings for the
// given serviceGlobalKey, without asserting they exist in the first place.
func removeEndpointBindingsOp(serviceGlobalKey string) txn.Op {
	return txn.Op{
		C:      endpointBindingsC,
		Id:     serviceGlobalKey,
		Remove: true,
	}
}
