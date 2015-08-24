// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stderrors "errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/leadership"
)

// Service represents the state of a service.
type Service struct {
	st  *State
	doc serviceDoc
}

// serviceDoc represents the internal state of a service in MongoDB.
// Note the correspondence with ServiceInfo in apiserver.
type serviceDoc struct {
	DocID             string     `bson:"_id"`
	Name              string     `bson:"name"`
	EnvUUID           string     `bson:"env-uuid"`
	Series            string     `bson:"series"`
	Subordinate       bool       `bson:"subordinate"`
	CharmURL          *charm.URL `bson:"charmurl"`
	ForceCharm        bool       `bson:"forcecharm"`
	Life              Life       `bson:"life"`
	UnitCount         int        `bson:"unitcount"`
	RelationCount     int        `bson:"relationcount"`
	Exposed           bool       `bson:"exposed"`
	MinUnits          int        `bson:"minunits"`
	OwnerTag          string     `bson:"ownertag"`
	TxnRevno          int64      `bson:"txn-revno"`
	MetricCredentials []byte     `bson:"metric-credentials"`
}

func newService(st *State, doc *serviceDoc) *Service {
	svc := &Service{
		st:  st,
		doc: *doc,
	}
	return svc
}

// Name returns the service name.
func (s *Service) Name() string {
	return s.doc.Name
}

// Tag returns a name identifying the service.
// The returned name will be different from other Tag values returned by any
// other entities from the same state.
func (s *Service) Tag() names.Tag {
	return names.NewServiceTag(s.Name())
}

// serviceGlobalKey returns the global database key for the service
// with the given name.
func serviceGlobalKey(svcName string) string {
	return "s#" + svcName
}

// globalKey returns the global database key for the service.
func (s *Service) globalKey() string {
	return serviceGlobalKey(s.doc.Name)
}

func serviceSettingsKey(serviceName string, curl *charm.URL) string {
	return fmt.Sprintf("s#%s#%s", serviceName, curl)
}

// settingsKey returns the charm-version-specific settings collection
// key for the service.
func (s *Service) settingsKey() string {
	return serviceSettingsKey(s.doc.Name, s.doc.CharmURL)
}

// Life returns whether the service is Alive, Dying or Dead.
func (s *Service) Life() Life {
	return s.doc.Life
}

var errRefresh = stderrors.New("state seems inconsistent, refresh and try again")

// Destroy ensures that the service and all its relations will be removed at
// some point; if the service has no units, and no relation involving the
// service has any units in scope, they are all removed immediately.
func (s *Service) Destroy() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy service %q", s)
	defer func() {
		if err == nil {
			// This is a white lie; the document might actually be removed.
			s.doc.Life = Dying
		}
	}()
	svc := &Service{st: s.st, doc: s.doc}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := svc.Refresh(); errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, err
			}
		}
		switch ops, err := svc.destroyOps(); err {
		case errRefresh:
		case errAlreadyDying:
			return nil, jujutxn.ErrNoOperations
		case nil:
			return ops, nil
		default:
			return nil, err
		}
		return nil, jujutxn.ErrTransientFailure
	}
	return s.st.run(buildTxn)
}

// destroyOps returns the operations required to destroy the service. If it
// returns errRefresh, the service should be refreshed and the destruction
// operations recalculated.
func (s *Service) destroyOps() ([]txn.Op, error) {
	if s.doc.Life == Dying {
		return nil, errAlreadyDying
	}
	rels, err := s.Relations()
	if err != nil {
		return nil, err
	}
	if len(rels) != s.doc.RelationCount {
		// This is just an early bail out. The relations obtained may still
		// be wrong, but that situation will be caught by a combination of
		// asserts on relationcount and on each known relation, below.
		return nil, errRefresh
	}
	ops := []txn.Op{minUnitsRemoveOp(s.st, s.doc.Name)}
	removeCount := 0
	for _, rel := range rels {
		relOps, isRemove, err := rel.destroyOps(s.doc.Name)
		if err == errAlreadyDying {
			relOps = []txn.Op{{
				C:      relationsC,
				Id:     rel.doc.DocID,
				Assert: bson.D{{"life", Dying}},
			}}
		} else if err != nil {
			return nil, err
		}
		if isRemove {
			removeCount++
		}
		ops = append(ops, relOps...)
	}
	// If the service has no units, and all its known relations will be
	// removed, the service can also be removed.
	if s.doc.UnitCount == 0 && s.doc.RelationCount == removeCount {
		hasLastRefs := bson.D{{"life", Alive}, {"unitcount", 0}, {"relationcount", removeCount}}
		return append(ops, s.removeOps(hasLastRefs)...), nil
	}
	// In all other cases, service removal will be handled as a consequence
	// of the removal of the last unit or relation referencing it. If any
	// relations have been removed, they'll be caught by the operations
	// collected above; but if any has been added, we need to abort and add
	// a destroy op for that relation too. In combination, it's enough to
	// check for count equality: an add/remove will not touch the count, but
	// will be caught by virtue of being a remove.
	notLastRefs := bson.D{
		{"life", Alive},
		{"relationcount", s.doc.RelationCount},
	}
	// With respect to unit count, a changing value doesn't matter, so long
	// as the count's equality with zero does not change, because all we care
	// about is that *some* unit is, or is not, keeping the service from
	// being removed: the difference between 1 unit and 1000 is irrelevant.
	if s.doc.UnitCount > 0 {
		ops = append(ops, s.st.newCleanupOp(cleanupUnitsForDyingService, s.doc.Name))
		notLastRefs = append(notLastRefs, bson.D{{"unitcount", bson.D{{"$gt", 0}}}}...)
	} else {
		notLastRefs = append(notLastRefs, bson.D{{"unitcount", 0}}...)
	}
	update := bson.D{{"$set", bson.D{{"life", Dying}}}}
	if removeCount != 0 {
		decref := bson.D{{"$inc", bson.D{{"relationcount", -removeCount}}}}
		update = append(update, decref...)
	}
	return append(ops, txn.Op{
		C:      servicesC,
		Id:     s.doc.DocID,
		Assert: notLastRefs,
		Update: update,
	}), nil
}

// removeOps returns the operations required to remove the service. Supplied
// asserts will be included in the operation on the service document.
func (s *Service) removeOps(asserts bson.D) []txn.Op {
	settingsDocID := s.st.docID(s.settingsKey())
	ops := []txn.Op{
		{
			C:      servicesC,
			Id:     s.doc.DocID,
			Assert: asserts,
			Remove: true,
		}, {
			C:      settingsrefsC,
			Id:     settingsDocID,
			Remove: true,
		}, {
			C:      settingsC,
			Id:     settingsDocID,
			Remove: true,
		},
		removeRequestedNetworksOp(s.st, s.globalKey()),
		removeStorageConstraintsOp(s.globalKey()),
		removeConstraintsOp(s.st, s.globalKey()),
		annotationRemoveOp(s.st, s.globalKey()),
		removeLeadershipSettingsOp(s.Tag().Id()),
		removeStatusOp(s.st, s.globalKey()),
	}
	return ops
}

// IsExposed returns whether this service is exposed. The explicitly open
// ports (with open-port) for exposed services may be accessed from machines
// outside of the local deployment network. See SetExposed and ClearExposed.
func (s *Service) IsExposed() bool {
	return s.doc.Exposed
}

// SetExposed marks the service as exposed.
// See ClearExposed and IsExposed.
func (s *Service) SetExposed() error {
	return s.setExposed(true)
}

// ClearExposed removes the exposed flag from the service.
// See SetExposed and IsExposed.
func (s *Service) ClearExposed() error {
	return s.setExposed(false)
}

func (s *Service) setExposed(exposed bool) (err error) {
	ops := []txn.Op{{
		C:      servicesC,
		Id:     s.doc.DocID,
		Assert: isAliveDoc,
		Update: bson.D{{"$set", bson.D{{"exposed", exposed}}}},
	}}
	if err := s.st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set exposed flag for service %q to %v: %v", s, exposed, onAbort(err, errNotAlive))
	}
	s.doc.Exposed = exposed
	return nil
}

// Charm returns the service's charm and whether units should upgrade to that
// charm even if they are in an error state.
func (s *Service) Charm() (ch *Charm, force bool, err error) {
	ch, err = s.st.Charm(s.doc.CharmURL)
	if err != nil {
		return nil, false, err
	}
	return ch, s.doc.ForceCharm, nil
}

// IsPrincipal returns whether units of the service can
// have subordinate units.
func (s *Service) IsPrincipal() bool {
	return !s.doc.Subordinate
}

// CharmURL returns the service's charm URL, and whether units should upgrade
// to the charm with that URL even if they are in an error state.
func (s *Service) CharmURL() (curl *charm.URL, force bool) {
	return s.doc.CharmURL, s.doc.ForceCharm
}

// Endpoints returns the service's currently available relation endpoints.
func (s *Service) Endpoints() (eps []Endpoint, err error) {
	ch, _, err := s.Charm()
	if err != nil {
		return nil, err
	}
	collect := func(role charm.RelationRole, rels map[string]charm.Relation) {
		for _, rel := range rels {
			eps = append(eps, Endpoint{
				ServiceName: s.doc.Name,
				Relation:    rel,
			})
		}
	}
	meta := ch.Meta()
	collect(charm.RolePeer, meta.Peers)
	collect(charm.RoleProvider, meta.Provides)
	collect(charm.RoleRequirer, meta.Requires)
	collect(charm.RoleProvider, map[string]charm.Relation{
		"juju-info": {
			Name:      "juju-info",
			Role:      charm.RoleProvider,
			Interface: "juju-info",
			Scope:     charm.ScopeGlobal,
		},
	})
	sort.Sort(epSlice(eps))
	return eps, nil
}

// Endpoint returns the relation endpoint with the supplied name, if it exists.
func (s *Service) Endpoint(relationName string) (Endpoint, error) {
	eps, err := s.Endpoints()
	if err != nil {
		return Endpoint{}, err
	}
	for _, ep := range eps {
		if ep.Name == relationName {
			return ep, nil
		}
	}
	return Endpoint{}, fmt.Errorf("service %q has no %q relation", s, relationName)
}

// extraPeerRelations returns only the peer relations in newMeta not
// present in the service's current charm meta data.
func (s *Service) extraPeerRelations(newMeta *charm.Meta) map[string]charm.Relation {
	if newMeta == nil {
		// This should never happen, since we're checking the charm in SetCharm already.
		panic("newMeta is nil")
	}
	ch, _, err := s.Charm()
	if err != nil {
		return nil
	}
	newPeers := newMeta.Peers
	oldPeers := ch.Meta().Peers
	extraPeers := make(map[string]charm.Relation)
	for relName, rel := range newPeers {
		if _, ok := oldPeers[relName]; !ok {
			extraPeers[relName] = rel
		}
	}
	return extraPeers
}

func (s *Service) checkRelationsOps(ch *Charm, relations []*Relation) ([]txn.Op, error) {
	asserts := make([]txn.Op, 0, len(relations))
	// All relations must still exist and their endpoints are implemented by the charm.
	for _, rel := range relations {
		if ep, err := rel.Endpoint(s.doc.Name); err != nil {
			return nil, err
		} else if !ep.ImplementedBy(ch) {
			return nil, fmt.Errorf("cannot upgrade service %q to charm %q: would break relation %q", s, ch, rel)
		}
		asserts = append(asserts, txn.Op{
			C:      relationsC,
			Id:     rel.doc.DocID,
			Assert: txn.DocExists,
		})
	}
	return asserts, nil
}

func (s *Service) checkStorageUpgrade(newMeta *charm.Meta) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot upgrade service %q to charm %q", s, newMeta.Name)
	ch, _, err := s.Charm()
	if err != nil {
		return errors.Trace(err)
	}
	oldMeta := ch.Meta()
	for name := range oldMeta.Storage {
		if _, ok := newMeta.Storage[name]; !ok {
			return errors.Errorf("storage %q removed", name)
		}
	}
	less := func(a, b int) bool {
		return a != -1 && (b == -1 || a < b)
	}
	for name, newStorageMeta := range newMeta.Storage {
		oldStorageMeta, ok := oldMeta.Storage[name]
		if !ok {
			if newStorageMeta.CountMin > 0 {
				return errors.Errorf("required storage %q added", name)
			}
			// New storage is fine as long as it is not required.
			//
			// TODO(axw) introduce a way of adding storage at
			// upgrade time. We should also look at supplying
			// a way of adding/changing other things during
			// upgrade, e.g. changing service config.
			continue
		}
		if newStorageMeta.Type != oldStorageMeta.Type {
			return errors.Errorf(
				"existing storage %q type changed from %q to %q",
				name, oldStorageMeta.Type, newStorageMeta.Type,
			)
		}
		if newStorageMeta.Shared != oldStorageMeta.Shared {
			return errors.Errorf(
				"existing storage %q shared changed from %v to %v",
				name, oldStorageMeta.Shared, newStorageMeta.Shared,
			)
		}
		if newStorageMeta.ReadOnly != oldStorageMeta.ReadOnly {
			return errors.Errorf(
				"existing storage %q read-only changed from %v to %v",
				name, oldStorageMeta.ReadOnly, newStorageMeta.ReadOnly,
			)
		}
		if newStorageMeta.Location != oldStorageMeta.Location {
			return errors.Errorf(
				"existing storage %q location changed from %q to %q",
				name, oldStorageMeta.Location, newStorageMeta.Location,
			)
		}
		if newStorageMeta.CountMin > oldStorageMeta.CountMin {
			return errors.Errorf(
				"existing storage %q range contracted: min increased from %d to %d",
				name, oldStorageMeta.CountMin, newStorageMeta.CountMin,
			)
		}
		if less(newStorageMeta.CountMax, oldStorageMeta.CountMax) {
			var oldCountMax interface{} = oldStorageMeta.CountMax
			if oldStorageMeta.CountMax == -1 {
				oldCountMax = "<unbounded>"
			}
			return errors.Errorf(
				"existing storage %q range contracted: max decreased from %v to %d",
				name, oldCountMax, newStorageMeta.CountMax,
			)
		}
		if oldStorageMeta.Location != "" && oldStorageMeta.CountMax == 1 && newStorageMeta.CountMax != 1 {
			// If a location is specified, the store may not go
			// from being a singleton to multiple, since then the
			// location has a different meaning.
			return errors.Errorf(
				"existing storage %q with location changed from singleton to multiple",
				name,
			)
		}
	}
	return nil
}

// changeCharmOps returns the operations necessary to set a service's
// charm URL to a new value.
func (s *Service) changeCharmOps(ch *Charm, force bool) ([]txn.Op, error) {
	// Build the new service config from what can be used of the old one.
	var newSettings charm.Settings
	oldSettings, err := readSettings(s.st, s.settingsKey())
	if err == nil {
		// Filter the old settings through to get the new settings.
		newSettings = ch.Config().FilterSettings(oldSettings.Map())
	} else if errors.IsNotFound(err) {
		// No old settings, start with empty new settings.
		newSettings = make(charm.Settings)
	} else {
		return nil, errors.Trace(err)
	}

	// Create or replace service settings.
	var settingsOp txn.Op
	newKey := serviceSettingsKey(s.doc.Name, ch.URL())
	if _, err := readSettings(s.st, newKey); errors.IsNotFound(err) {
		// No settings for this key yet, create it.
		settingsOp = createSettingsOp(s.st, newKey, newSettings)
	} else if err != nil {
		return nil, errors.Trace(err)
	} else {
		// Settings exist, just replace them with the new ones.
		settingsOp, _, err = replaceSettingsOp(s.st, newKey, newSettings)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	// Add or create a reference to the new settings doc.
	incOp, err := settingsIncRefOp(s.st, s.doc.Name, ch.URL(), true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var decOps []txn.Op
	// Drop the reference to the old settings doc (if they exist).
	if oldSettings != nil {
		decOps, err = settingsDecRefOps(s.st, s.doc.Name, s.doc.CharmURL) // current charm
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	// Build the transaction.
	var ops []txn.Op
	differentCharm := bson.D{{"charmurl", bson.D{{"$ne", ch.URL()}}}}
	if oldSettings != nil {
		// Old settings shouldn't change (when they exist).
		ops = append(ops, oldSettings.assertUnchangedOp())
	}
	ops = append(ops, []txn.Op{
		// Create or replace new settings.
		settingsOp,
		// Increment the ref count.
		incOp,
		// Update the charm URL and force flag (if relevant).
		{
			C:      servicesC,
			Id:     s.doc.DocID,
			Assert: append(notDeadDoc, differentCharm...),
			Update: bson.D{{"$set", bson.D{{"charmurl", ch.URL()}, {"forcecharm", force}}}},
		},
	}...)
	// Add any extra peer relations that need creation.
	newPeers := s.extraPeerRelations(ch.Meta())
	peerOps, err := s.st.addPeerRelationsOps(s.doc.Name, newPeers)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Get all relations - we need to check them later.
	relations, err := s.Relations()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Make sure the relation count does not change.
	sameRelCount := bson.D{{"relationcount", len(relations)}}

	ops = append(ops, peerOps...)
	// Update the relation count as well.
	ops = append(ops, txn.Op{
		C:      servicesC,
		Id:     s.doc.DocID,
		Assert: append(notDeadDoc, sameRelCount...),
		Update: bson.D{{"$inc", bson.D{{"relationcount", len(newPeers)}}}},
	})
	// Check relations to ensure no active relations are removed.
	relOps, err := s.checkRelationsOps(ch, relations)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, relOps...)

	// Check storage to ensure no storage is removed, and no required
	// storage is added for which there are no constraints.
	if err := s.checkStorageUpgrade(ch.Meta()); err != nil {
		return nil, errors.Trace(err)
	}

	// And finally, decrement the old settings.
	return append(ops, decOps...), nil
}

// SetCharm changes the charm for the service. New units will be started with
// this charm, and existing units will be upgraded to use it. If force is true,
// units will be upgraded even if they are in an error state.
func (s *Service) SetCharm(ch *Charm, force bool) error {
	if ch.Meta().Subordinate != s.doc.Subordinate {
		return errors.Errorf("cannot change a service's subordinacy")
	}
	if ch.URL().Series != s.doc.Series {
		return errors.Errorf("cannot change a service's series")
	}

	services, closer := s.st.getCollection(servicesC)
	defer closer()

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			// NOTE: We're explicitly allowing SetCharm to succeed
			// when the service is Dying, because service/charm
			// upgrades should still be allowed to apply to dying
			// services and units, so that bugs in departed/broken
			// hooks can be addressed at runtime.
			if notDead, err := isNotDeadWithSession(services, s.doc.DocID); err != nil {
				return nil, errors.Trace(err)
			} else if !notDead {
				return nil, ErrDead
			}
		}
		// Make sure the service doesn't have this charm already.
		sel := bson.D{{"_id", s.doc.DocID}, {"charmurl", ch.URL()}}
		var ops []txn.Op
		if count, err := services.Find(sel).Count(); err != nil {
			return nil, errors.Trace(err)
		} else if count == 1 {
			// Charm URL already set; just update the force flag.
			sameCharm := bson.D{{"charmurl", ch.URL()}}
			ops = []txn.Op{{
				C:      servicesC,
				Id:     s.doc.DocID,
				Assert: append(notDeadDoc, sameCharm...),
				Update: bson.D{{"$set", bson.D{{"forcecharm", force}}}},
			}}
		} else {
			// Change the charm URL.
			ops, err = s.changeCharmOps(ch, force)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		return ops, nil
	}
	err := s.st.run(buildTxn)
	if err == nil {
		s.doc.CharmURL = ch.URL()
		s.doc.ForceCharm = force
	}
	return err
}

// String returns the service name.
func (s *Service) String() string {
	return s.doc.Name
}

// Refresh refreshes the contents of the Service from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// service has been removed.
func (s *Service) Refresh() error {
	services, closer := s.st.getCollection(servicesC)
	defer closer()

	err := services.FindId(s.doc.DocID).One(&s.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("service %q", s)
	}
	if err != nil {
		return fmt.Errorf("cannot refresh service %q: %v", s, err)
	}
	return nil
}

// newUnitName returns the next unit name.
func (s *Service) newUnitName() (string, error) {
	services, closer := s.st.getCollection(servicesC)
	defer closer()

	result := &serviceDoc{}
	if err := services.FindId(s.doc.DocID).One(&result); err == mgo.ErrNotFound {
		return "", errors.NotFoundf("service %q", s)
	}

	unitSeq, err := s.st.sequence(s.Tag().String())
	if err != nil {
		return "", errors.Trace(err)
	}
	name := s.doc.Name + "/" + strconv.Itoa(unitSeq)
	return name, nil
}

const MessageWaitForAgentInit = "Waiting for agent initialization to finish"

// addUnitOps returns a unique name for a new unit, and a list of txn operations
// necessary to create that unit. The principalName param must be non-empty if
// and only if s is a subordinate service. Only one subordinate of a given
// service will be assigned to a given principal. The asserts param can be used
// to include additional assertions for the service document.
func (s *Service) addUnitOps(principalName string, asserts bson.D) (string, []txn.Op, error) {
	if s.doc.Subordinate && principalName == "" {
		return "", nil, fmt.Errorf("service is a subordinate")
	} else if !s.doc.Subordinate && principalName != "" {
		return "", nil, fmt.Errorf("service is not a subordinate")
	}
	name, err := s.newUnitName()
	if err != nil {
		return "", nil, err
	}

	// Create instances of the charm's declared stores.
	storageOps, numStorageAttachments, err := s.unitStorageOps(name)
	if err != nil {
		return "", nil, errors.Trace(err)
	}

	docID := s.st.docID(name)
	globalKey := unitGlobalKey(name)
	agentGlobalKey := unitAgentGlobalKey(name)
	meterStatusGlobalKey := unitAgentGlobalKey(name)
	udoc := &unitDoc{
		DocID:                  docID,
		Name:                   name,
		EnvUUID:                s.doc.EnvUUID,
		Service:                s.doc.Name,
		Series:                 s.doc.Series,
		Life:                   Alive,
		Principal:              principalName,
		StorageAttachmentCount: numStorageAttachments,
	}
	now := time.Now()
	agentStatusDoc := statusDoc{
		Status:  StatusAllocating,
		Updated: now.UnixNano(),
		EnvUUID: s.st.EnvironUUID(),
	}
	unitStatusDoc := statusDoc{
		// TODO(fwereade): this violates the spec. Should be "waiting".
		Status:     StatusUnknown,
		StatusInfo: MessageWaitForAgentInit,
		Updated:    now.UnixNano(),
		EnvUUID:    s.st.EnvironUUID(),
	}
	ops := []txn.Op{
		createStatusOp(s.st, globalKey, unitStatusDoc),
		createStatusOp(s.st, agentGlobalKey, agentStatusDoc),
		createMeterStatusOp(s.st, meterStatusGlobalKey, &meterStatusDoc{Code: MeterNotSet.String()}),
		{
			C:      unitsC,
			Id:     docID,
			Assert: txn.DocMissing,
			Insert: udoc,
		},
		{
			C:      servicesC,
			Id:     s.doc.DocID,
			Assert: append(isAliveDoc, asserts...),
			Update: bson.D{{"$inc", bson.D{{"unitcount", 1}}}},
		},
	}
	ops = append(ops, storageOps...)

	if s.doc.Subordinate {
		ops = append(ops, txn.Op{
			C:  unitsC,
			Id: s.st.docID(principalName),
			Assert: append(isAliveDoc, bson.DocElem{
				"subordinates", bson.D{{"$not", bson.RegEx{Pattern: "^" + s.doc.Name + "/"}}},
			}),
			Update: bson.D{{"$addToSet", bson.D{{"subordinates", name}}}},
		})
	} else {
		scons, err := s.Constraints()
		if err != nil {
			return "", nil, err
		}
		cons, err := s.st.resolveConstraints(scons)
		if err != nil {
			return "", nil, err
		}
		ops = append(ops, createConstraintsOp(s.st, agentGlobalKey, cons))
	}

	// At the last moment we still have the statusDocs in scope, set the initial
	// history entries. This is risky, and may lead to extra entries, but that's
	// an intrinsic problem with mixing txn and non-txn ops -- we can't sync
	// them cleanly.
	probablyUpdateStatusHistory(s.st, globalKey, unitStatusDoc)
	probablyUpdateStatusHistory(s.st, agentGlobalKey, agentStatusDoc)
	return name, ops, nil
}

// unitStorageOps returns operations for creating storage
// instances and attachments for a new unit. unitStorageOps
// returns the number of initial storage attachments, to
// initialise the unit's storage attachment refcount.
func (s *Service) unitStorageOps(unitName string) (ops []txn.Op, numStorageAttachments int, err error) {
	cons, err := s.StorageConstraints()
	if err != nil {
		return nil, -1, err
	}
	charm, _, err := s.Charm()
	if err != nil {
		return nil, -1, err
	}
	meta := charm.Meta()
	url := charm.URL()
	tag := names.NewUnitTag(unitName)
	// TODO(wallyworld) - record constraints info in data model - size and pool name
	ops, numStorageAttachments, err = createStorageOps(
		s.st, tag, meta, url, cons,
		s.doc.Series,
		false, // unit is not assigned yet; don't create machine storage
	)
	if err != nil {
		return nil, -1, errors.Trace(err)
	}
	return ops, numStorageAttachments, nil
}

// SCHEMACHANGE
// TODO(mattyw) remove when schema upgrades are possible
func (s *Service) GetOwnerTag() string {
	owner := s.doc.OwnerTag
	if owner == "" {
		// We know that if there was no owner, it was created with an early
		// version of juju, and that admin was the only user.
		owner = names.NewUserTag("admin").String()
	}
	return owner
}

// AddUnit adds a new principal unit to the service.
func (s *Service) AddUnit() (unit *Unit, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add unit to service %q", s)
	name, ops, err := s.addUnitOps("", nil)
	if err != nil {
		return nil, err
	}
	if err := s.st.runTransaction(ops); err == txn.ErrAborted {
		if alive, err := isAlive(s.st, servicesC, s.doc.DocID); err != nil {
			return nil, err
		} else if !alive {
			return nil, fmt.Errorf("service is not alive")
		}
		return nil, fmt.Errorf("inconsistent state")
	} else if err != nil {
		return nil, err
	}
	return s.st.Unit(name)
}

// removeUnitOps returns the operations necessary to remove the supplied unit,
// assuming the supplied asserts apply to the unit document.
func (s *Service) removeUnitOps(u *Unit, asserts bson.D) ([]txn.Op, error) {
	ops, err := u.destroyHostOps(s)
	if err != nil {
		return nil, err
	}
	portsOps, err := removePortsForUnitOps(s.st, u)
	if err != nil {
		return nil, err
	}
	storageInstanceOps, err := removeStorageInstancesOps(s.st, u.Tag())
	if err != nil {
		return nil, err
	}

	observedFieldsMatch := bson.D{
		{"charmurl", u.doc.CharmURL},
		{"machineid", u.doc.MachineId},
	}
	ops = append(ops,
		txn.Op{
			C:      unitsC,
			Id:     u.doc.DocID,
			Assert: append(observedFieldsMatch, asserts...),
			Remove: true,
		},
		removeMeterStatusOp(s.st, u.globalMeterStatusKey()),
		removeStatusOp(s.st, u.globalAgentKey()),
		removeStatusOp(s.st, u.globalKey()),
		removeConstraintsOp(s.st, u.globalAgentKey()),
		annotationRemoveOp(s.st, u.globalKey()),
		s.st.newCleanupOp(cleanupRemovedUnit, u.doc.Name),
	)
	ops = append(ops, portsOps...)
	ops = append(ops, storageInstanceOps...)
	if u.doc.CharmURL != nil {
		decOps, err := settingsDecRefOps(s.st, s.doc.Name, u.doc.CharmURL)
		if errors.IsNotFound(err) {
			return nil, errRefresh
		} else if err != nil {
			return nil, err
		}
		ops = append(ops, decOps...)
	}
	if s.doc.Life == Dying && s.doc.RelationCount == 0 && s.doc.UnitCount == 1 {
		hasLastRef := bson.D{{"life", Dying}, {"relationcount", 0}, {"unitcount", 1}}
		return append(ops, s.removeOps(hasLastRef)...), nil
	}
	svcOp := txn.Op{
		C:      servicesC,
		Id:     s.doc.DocID,
		Update: bson.D{{"$inc", bson.D{{"unitcount", -1}}}},
	}
	if s.doc.Life == Alive {
		svcOp.Assert = bson.D{{"life", Alive}, {"unitcount", bson.D{{"$gt", 0}}}}
	} else {
		svcOp.Assert = bson.D{
			{"life", Dying},
			{"$or", []bson.D{
				{{"unitcount", bson.D{{"$gt", 1}}}},
				{{"relationcount", bson.D{{"$gt", 0}}}},
			}},
		}
	}
	ops = append(ops, svcOp)

	return ops, nil
}

// AllUnits returns all units of the service.
func (s *Service) AllUnits() (units []*Unit, err error) {
	return allUnits(s.st, s.doc.Name)
}

func allUnits(st *State, service string) (units []*Unit, err error) {
	unitsCollection, closer := st.getCollection(unitsC)
	defer closer()

	docs := []unitDoc{}
	err = unitsCollection.Find(bson.D{{"service", service}}).All(&docs)
	if err != nil {
		return nil, fmt.Errorf("cannot get all units from service %q: %v", service, err)
	}
	for i := range docs {
		units = append(units, newUnit(st, &docs[i]))
	}
	return units, nil
}

// Relations returns a Relation for every relation the service is in.
func (s *Service) Relations() (relations []*Relation, err error) {
	return serviceRelations(s.st, s.doc.Name)
}

func serviceRelations(st *State, name string) (relations []*Relation, err error) {
	defer errors.DeferredAnnotatef(&err, "can't get relations for service %q", name)
	relationsCollection, closer := st.getCollection(relationsC)
	defer closer()

	docs := []relationDoc{}
	err = relationsCollection.Find(bson.D{{"endpoints.servicename", name}}).All(&docs)
	if err != nil {
		return nil, err
	}
	for _, v := range docs {
		relations = append(relations, newRelation(st, &v))
	}
	return relations, nil
}

// ConfigSettings returns the raw user configuration for the service's charm.
// Unset values are omitted.
func (s *Service) ConfigSettings() (charm.Settings, error) {
	settings, err := readSettings(s.st, s.settingsKey())
	if err != nil {
		return nil, err
	}
	return settings.Map(), nil
}

// UpdateConfigSettings changes a service's charm config settings. Values set
// to nil will be deleted; unknown and invalid values will return an error.
func (s *Service) UpdateConfigSettings(changes charm.Settings) error {
	charm, _, err := s.Charm()
	if err != nil {
		return err
	}
	changes, err = charm.Config().ValidateSettings(changes)
	if err != nil {
		return err
	}
	// TODO(fwereade) state.Settings is itself really problematic in just
	// about every use case. This needs to be resolved some time; but at
	// least the settings docs are keyed by charm url as well as service
	// name, so the actual impact of a race is non-threatening.
	node, err := readSettings(s.st, s.settingsKey())
	if err != nil {
		return err
	}
	for name, value := range changes {
		if value == nil {
			node.Delete(name)
		} else {
			node.Set(name, value)
		}
	}
	_, err = node.Write()
	return err
}

// LeaderSettings returns a service's leader settings. If nothing has been set
// yet, it will return an empty map; this is not an error.
func (s *Service) LeaderSettings() (map[string]string, error) {
	// There's no compelling reason to have these methods on Service -- and
	// thus require an extra db read to access them -- but it stops the State
	// type getting even more cluttered.

	docId := leadershipSettingsDocId(s.doc.Name)
	rawMap, _, err := readSettingsDoc(s.st, docId)
	if cause := errors.Cause(err); cause == mgo.ErrNotFound {
		return nil, errors.NotFoundf("service")
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	result := make(map[string]string)
	for escapedKey, interfaceValue := range rawMap {
		key := unescapeReplacer.Replace(escapedKey)
		if value, _ := interfaceValue.(string); value != "" {
			// Empty strings are technically bad data -- when set, they clear.
			result[key] = value
		} else {
			// Some bad data isn't reason enough to obscure the good data.
			logger.Warningf("unexpected leader settings value for %s: %#v", key, interfaceValue)
		}
	}
	return result, nil
}

// UpdateLeaderSettings updates the service's leader settings with the supplied
// values, but will fail (with a suitable error) if the supplied Token loses
// validity. Empty values in the supplied map will be cleared in the database.
func (s *Service) UpdateLeaderSettings(token leadership.Token, updates map[string]string) error {
	// There's no compelling reason to have these methods on Service -- and
	// thus require an extra db read to access them -- but it stops the State
	// type getting even more cluttered.

	// We can calculate the actual update ahead of time; it's not dependent
	// upon the current state of the document. (*Writing* it should depend
	// on document state, but that's handled below.)
	docId := leadershipSettingsDocId(s.doc.Name)
	sets := bson.M{}
	unsets := bson.M{}
	for unescapedKey, value := range updates {
		key := escapeReplacer.Replace(unescapedKey)
		if value == "" {
			unsets[key] = 1
		} else {
			sets[key] = value
		}
	}
	update := setUnsetUpdate(sets, unsets)

	isNullChange := func(rawMap map[string]interface{}) bool {
		for key := range unsets {
			if _, found := rawMap[key]; found {
				return false
			}
		}
		for key, value := range sets {
			if current := rawMap[key]; current != value {
				return false
			}
		}
		return true
	}

	buildTxn := func(_ int) ([]txn.Op, error) {
		// Read the current document state so we can abort if there's
		// no actual change; and the txn-revno so we can assert on it
		// and prevent these settings from landing late.
		rawMap, txnRevno, err := readSettingsDoc(s.st, docId)
		if cause := errors.Cause(err); cause == mgo.ErrNotFound {
			return nil, errors.NotFoundf("service")
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if isNullChange(rawMap) {
			return nil, jujutxn.ErrNoOperations
		}
		return []txn.Op{{
			C:      settingsC,
			Id:     docId,
			Assert: bson.D{{"txn-revno", txnRevno}},
			Update: update,
		}}, nil
	}
	return s.st.run(buildTxnWithLeadership(buildTxn, token))
}

var ErrSubordinateConstraints = stderrors.New("constraints do not apply to subordinate services")

// Constraints returns the current service constraints.
func (s *Service) Constraints() (constraints.Value, error) {
	if s.doc.Subordinate {
		return constraints.Value{}, ErrSubordinateConstraints
	}
	return readConstraints(s.st, s.globalKey())
}

// SetConstraints replaces the current service constraints.
func (s *Service) SetConstraints(cons constraints.Value) (err error) {
	unsupported, err := s.st.validateConstraints(cons)
	if len(unsupported) > 0 {
		logger.Warningf(
			"setting constraints on service %q: unsupported constraints: %v", s.Name(), strings.Join(unsupported, ","))
	} else if err != nil {
		return err
	}
	if s.doc.Subordinate {
		return ErrSubordinateConstraints
	}
	defer errors.DeferredAnnotatef(&err, "cannot set constraints")
	if s.doc.Life != Alive {
		return errNotAlive
	}
	ops := []txn.Op{{
		C:      servicesC,
		Id:     s.doc.DocID,
		Assert: isAliveDoc,
	}}
	ops = append(ops, setConstraintsOp(s.st, s.globalKey(), cons))
	return onAbort(s.st.runTransaction(ops), errNotAlive)
}

// Networks returns the networks a service is associated with. Unlike
// networks specified with constraints, these networks are required to
// be present on machines hosting this service's units.
//
// TODO(dimitern): Convert this to use spaces from constraints or drop
// it entirely.
func (s *Service) Networks() ([]string, error) {
	return readRequestedNetworks(s.st, s.globalKey())
}

// MetricCredentials returns any metric credentials associated with this service.
func (s *Service) MetricCredentials() []byte {
	return s.doc.MetricCredentials
}

// SetMetricCredentials updates the metric credentials associated with this service.
func (s *Service) SetMetricCredentials(b []byte) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			alive, err := isAlive(s.st, servicesC, s.doc.DocID)
			if err != nil {
				return nil, errors.Trace(err)
			} else if !alive {
				return nil, errNotAlive
			}
		}
		ops := []txn.Op{
			{
				C:      servicesC,
				Id:     s.doc.DocID,
				Assert: isAliveDoc,
				Update: bson.M{"$set": bson.M{"metric-credentials": b}},
			},
		}
		return ops, nil
	}
	if err := s.st.run(buildTxn); err != nil {
		if err == errNotAlive {
			return errors.New("cannot update metric credentials: service " + err.Error())
		}
		return errors.Annotatef(err, "cannot update metric credentials")
	}
	s.doc.MetricCredentials = b
	return nil
}

func (s *Service) StorageConstraints() (map[string]StorageConstraints, error) {
	return readStorageConstraints(s.st, s.globalKey())
}

// settingsIncRefOp returns an operation that increments the ref count
// of the service settings identified by serviceName and curl. If
// canCreate is false, a missing document will be treated as an error;
// otherwise, it will be created with a ref count of 1.
func settingsIncRefOp(st *State, serviceName string, curl *charm.URL, canCreate bool) (txn.Op, error) {
	settingsrefs, closer := st.getCollection(settingsrefsC)
	defer closer()

	key := serviceSettingsKey(serviceName, curl)
	if count, err := settingsrefs.FindId(key).Count(); err != nil {
		return txn.Op{}, err
	} else if count == 0 {
		if !canCreate {
			return txn.Op{}, errors.NotFoundf("service %q settings for charm %q", serviceName, curl)
		}
		return txn.Op{
			C:      settingsrefsC,
			Id:     st.docID(key),
			Assert: txn.DocMissing,
			Insert: settingsRefsDoc{
				RefCount: 1,
				EnvUUID:  st.EnvironUUID()},
		}, nil
	}
	return txn.Op{
		C:      settingsrefsC,
		Id:     st.docID(key),
		Assert: txn.DocExists,
		Update: bson.D{{"$inc", bson.D{{"refcount", 1}}}},
	}, nil
}

// settingsDecRefOps returns a list of operations that decrement the
// ref count of the service settings identified by serviceName and
// curl. If the ref count is set to zero, the appropriate setting and
// ref count documents will both be deleted.
func settingsDecRefOps(st *State, serviceName string, curl *charm.URL) ([]txn.Op, error) {
	settingsrefs, closer := st.getCollection(settingsrefsC)
	defer closer()

	key := serviceSettingsKey(serviceName, curl)
	var doc settingsRefsDoc
	if err := settingsrefs.FindId(key).One(&doc); err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("service %q settings for charm %q", serviceName, curl)
	} else if err != nil {
		return nil, err
	}
	docID := st.docID(key)
	if doc.RefCount == 1 {
		return []txn.Op{{
			C:      settingsrefsC,
			Id:     docID,
			Assert: bson.D{{"refcount", 1}},
			Remove: true,
		}, {
			C:      settingsC,
			Id:     docID,
			Remove: true,
		}}, nil
	}
	return []txn.Op{{
		C:      settingsrefsC,
		Id:     docID,
		Assert: bson.D{{"refcount", bson.D{{"$gt", 1}}}},
		Update: bson.D{{"$inc", bson.D{{"refcount", -1}}}},
	}}, nil
}

// settingsRefsDoc holds the number of units and services using the
// settings document identified by the document's id. Every time a
// service upgrades its charm the settings doc ref count for the new
// charm url is incremented, and the old settings is ref count is
// decremented. When a unit upgrades to the new charm, the old service
// settings ref count is decremented and the ref count of the new
// charm settings is incremented. The last unit upgrading to the new
// charm is responsible for deleting the old charm's settings doc.
//
// Note: We're not using the settingsDoc for this because changing
// just the ref count is not considered a change worth reporting
// to watchers and firing config-changed hooks.
//
// There is an implicit _id field here, which mongo creates, which is
// always the same as the settingsDoc's id.
type settingsRefsDoc struct {
	RefCount int
	EnvUUID  string `bson:"env-uuid"`
}

// Status returns the status of the service.
// Only unit leaders are allowed to set the status of the service.
// If no status is recorded, then there are no unit leaders and the
// status is derived from the unit status values.
func (s *Service) Status() (StatusInfo, error) {
	statuses, closer := s.st.getCollection(statusesC)
	defer closer()
	query := statuses.Find(bson.D{{"_id", s.globalKey()}, {"neverset", true}})
	if count, err := query.Count(); err != nil {
		return StatusInfo{}, errors.Trace(err)
	} else if count != 0 {
		// This indicates that SetStatus has never been called on this service.
		// This in turn implies the service status document is likely to be
		// inaccurate, so we return aggregated unit statuses instead.
		//
		// TODO(fwereade): this is completely wrong and will produce bad results
		// in not-very-challenging scenarios. The leader unit remains responsible
		// for setting the service status in a timely way, *whether or not the
		// charm's hooks exists or sets a service status*. This logic should be
		// removed as soon as possible, and the responsibilities implemented in
		// the right places rather than being applied at seeming random.
		units, err := s.AllUnits()
		if err != nil {
			return StatusInfo{}, err
		}
		logger.Tracef("service %q has %d units", s.Name(), len(units))
		if len(units) > 0 {
			return s.deriveStatus(units)
		}
	}
	return getStatus(s.st, s.globalKey(), "service")
}

// SetStatus sets the status for the service.
func (s *Service) SetStatus(status Status, info string, data map[string]interface{}) error {
	if !ValidWorkloadStatus(status) {
		return errors.Errorf("cannot set invalid status %q", status)
	}
	return setStatus(s.st, setStatusParams{
		badge:     "service",
		globalKey: s.globalKey(),
		status:    status,
		message:   info,
		rawData:   data,
	})
}

// ServiceAndUnitsStatus returns the status for this service and all its units.
func (s *Service) ServiceAndUnitsStatus() (StatusInfo, map[string]StatusInfo, error) {
	serviceStatus, err := s.Status()
	if err != nil {
		return StatusInfo{}, nil, errors.Trace(err)
	}
	units, err := s.AllUnits()
	if err != nil {
		return StatusInfo{}, nil, err
	}
	results := make(map[string]StatusInfo, len(units))
	for _, unit := range units {
		unitStatus, err := unit.Status()
		if err != nil {
			return StatusInfo{}, nil, err
		}
		results[unit.Name()] = unitStatus
	}
	return serviceStatus, results, nil

}

func (s *Service) deriveStatus(units []*Unit) (StatusInfo, error) {
	var result StatusInfo
	for _, unit := range units {
		currentSeverity := statusServerities[result.Status]
		unitStatus, err := unit.Status()
		if err != nil {
			return StatusInfo{}, errors.Annotatef(err, "deriving service status from %q", unit.Name())
		}
		unitSeverity := statusServerities[unitStatus.Status]
		if unitSeverity > currentSeverity {
			result.Status = unitStatus.Status
			result.Message = unitStatus.Message
			result.Data = unitStatus.Data
			result.Since = unitStatus.Since
		}
	}
	return result, nil
}

// statusSeverities holds status values with a severity measure.
// Status values with higher severity are used in preference to others.
var statusServerities = map[Status]int{
	StatusError:       100,
	StatusBlocked:     90,
	StatusWaiting:     80,
	StatusMaintenance: 70,
	StatusTerminated:  60,
	StatusActive:      50,
	StatusUnknown:     40,
}
