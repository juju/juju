// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v2-unstable"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
)

// RemoteApplication represents the state of an application hosted
// in an external (remote) model.
type RemoteApplication struct {
	st  *State
	doc remoteApplicationDoc
}

// remoteApplicationDoc represents the internal state of a remote application in MongoDB.
type remoteApplicationDoc struct {
	DocID           string              `bson:"_id"`
	Name            string              `bson:"name"`
	OfferUUID       string              `bson:"offer-uuid"`
	URL             string              `bson:"url,omitempty"`
	SourceModelUUID string              `bson:"source-model-uuid"`
	Endpoints       []remoteEndpointDoc `bson:"endpoints"`
	Spaces          []remoteSpaceDoc    `bson:"spaces"`
	Bindings        map[string]string   `bson:"bindings"`
	Life            Life                `bson:"life"`
	RelationCount   int                 `bson:"relationcount"`
	IsConsumerProxy bool                `bson:"is-consumer-proxy"`
	Macaroon        string              `bson:"macaroon,omitempty"`
}

// remoteEndpointDoc represents the internal state of a remote application endpoint in MongoDB.
type remoteEndpointDoc struct {
	Name      string              `bson:"name"`
	Role      charm.RelationRole  `bson:"role"`
	Interface string              `bson:"interface"`
	Limit     int                 `bson:"limit"`
	Scope     charm.RelationScope `bson:"scope"`
}

type attributeMap map[string]interface{}

// remoteSpaceDoc represents the internal state of a space in another
// model in the DB.
type remoteSpaceDoc struct {
	CloudType          string            `bson:"cloud-type"`
	Name               string            `bson:"name"`
	ProviderId         string            `bson:"provider-id"`
	ProviderAttributes attributeMap      `bson:"provider-attributes"`
	Subnets            []remoteSubnetDoc `bson:"subnets"`
}

// RemoteSpace represents a space in another model that endpoints are
// bound to.
type RemoteSpace struct {
	CloudType          string
	Name               string
	ProviderId         string
	ProviderAttributes attributeMap
	Subnets            []RemoteSubnet
}

// remoteSubnetDoc represents a subnet in another model in the DB.
type remoteSubnetDoc struct {
	CIDR              string   `bson:"cidr"`
	ProviderId        string   `bson:"provider-id"`
	VLANTag           int      `bson:"vlan-tag"`
	AvailabilityZones []string `bson:"availability-zones"`
	ProviderSpaceId   string   `bson:"provider-space-id"`
	ProviderNetworkId string   `bson:"provider-network-id"`
}

// RemoteSubnet represents a subnet in another model.
type RemoteSubnet struct {
	CIDR              string
	ProviderId        string
	VLANTag           int
	AvailabilityZones []string
	ProviderSpaceId   string
	ProviderNetworkId string
}

func newRemoteApplication(st *State, doc *remoteApplicationDoc) *RemoteApplication {
	app := &RemoteApplication{
		st:  st,
		doc: *doc,
	}
	return app
}

// remoteApplicationGlobalKey returns the global database key for the
// remote application with the given name.
//
// This seems like an aggressively cryptic prefix, but apparently the
// all-watcher requires that global keys have single letter prefixes
// and r and a were taken.
// TODO(babbageclunk): check whether this is still the case.
func remoteApplicationGlobalKey(appName string) string {
	return "c#" + appName
}

// globalKey returns the global database key for the remote application.
func (s *RemoteApplication) globalKey() string {
	return remoteApplicationGlobalKey(s.doc.Name)
}

// IsRemote returns true for a remote application.
func (s *RemoteApplication) IsRemote() bool {
	return true
}

// SourceModel returns the tag of the model to which the application belongs.
func (s *RemoteApplication) SourceModel() names.ModelTag {
	return names.NewModelTag(s.doc.SourceModelUUID)
}

// IsConsumerProxy returns the application is created
// from a registration operation by a consuming model.
func (s *RemoteApplication) IsConsumerProxy() bool {
	return s.doc.IsConsumerProxy
}

// Name returns the application name.
func (s *RemoteApplication) Name() string {
	return s.doc.Name
}

// OfferUUID returns the offer UUID.
func (s *RemoteApplication) OfferUUID() string {
	return s.doc.OfferUUID
}

// URL returns the remote application URL, and a boolean indicating whether or not
// a URL is known for the remote application. A URL will only be available for the
// consumer of an offered application.
func (s *RemoteApplication) URL() (string, bool) {
	return s.doc.URL, s.doc.URL != ""
}

// Token returns the token for the remote application, provided by the remote
// model to identify the application in future communications.
func (s *RemoteApplication) Token() (string, error) {
	r := s.st.RemoteEntities()
	return r.GetToken(s.Tag())
}

// Tag returns a name identifying the application.
func (s *RemoteApplication) Tag() names.Tag {
	return names.NewApplicationTag(s.Name())
}

// Life returns whether the application is Alive, Dying or Dead.
func (s *RemoteApplication) Life() Life {
	return s.doc.Life
}

// Spaces returns the remote spaces this application is connected to.
func (s *RemoteApplication) Spaces() []RemoteSpace {
	var result []RemoteSpace
	for _, space := range s.doc.Spaces {
		result = append(result, remoteSpaceFromDoc(space))
	}
	return result
}

// Bindings returns the endpoint->space bindings for the application.
func (s *RemoteApplication) Bindings() map[string]string {
	result := make(map[string]string)
	for epName, spName := range s.doc.Bindings {
		result[epName] = spName
	}
	return result
}

// SpaceForEndpoint returns the remote space an endpoint is bound to,
// if one is found.
func (s *RemoteApplication) SpaceForEndpoint(endpointName string) (RemoteSpace, bool) {
	spaceName, ok := s.doc.Bindings[endpointName]
	if !ok {
		return RemoteSpace{}, false
	}
	for _, space := range s.doc.Spaces {
		if space.Name == spaceName {
			return remoteSpaceFromDoc(space), true
		}
	}
	return RemoteSpace{}, false
}

func remoteSpaceFromDoc(space remoteSpaceDoc) RemoteSpace {
	result := RemoteSpace{
		CloudType:          space.CloudType,
		Name:               space.Name,
		ProviderId:         space.ProviderId,
		ProviderAttributes: copyAttributes(space.ProviderAttributes),
	}
	for _, subnet := range space.Subnets {
		result.Subnets = append(result.Subnets, remoteSubnetFromDoc(subnet))
	}
	return result
}

func remoteSubnetFromDoc(subnet remoteSubnetDoc) RemoteSubnet {
	return RemoteSubnet{
		CIDR:              subnet.CIDR,
		ProviderId:        subnet.ProviderId,
		VLANTag:           subnet.VLANTag,
		AvailabilityZones: copyStrings(subnet.AvailabilityZones),
		ProviderSpaceId:   subnet.ProviderSpaceId,
		ProviderNetworkId: subnet.ProviderNetworkId,
	}
}

func copyStrings(values []string) []string {
	if values == nil {
		return nil
	}
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func copyAttributes(values attributeMap) attributeMap {
	if values == nil {
		return nil
	}
	result := make(attributeMap)
	for key, value := range values {
		result[key] = value
	}
	return result
}

// DestroyOperation returns a model operation to destroy remote application.
func (s *RemoteApplication) DestroyOperation(force bool) *DestroyRemoteApplicationOperation {
	return &DestroyRemoteApplicationOperation{
		app:             &RemoteApplication{st: s.st, doc: s.doc},
		ForcedOperation: ForcedOperation{Force: force},
	}
}

// DestroyRemoteApplicationOperation is a model operation to destroy a remote application.
type DestroyRemoteApplicationOperation struct {
	// ForcedOperation stores needed information to force this operation.
	ForcedOperation

	// app holds the remote application to destroy.
	app *RemoteApplication
}

// Build is part of the ModelOperation interface.
func (op *DestroyRemoteApplicationOperation) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		if err := op.app.Refresh(); errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, err
		}
	}
	// When 'force' is set on the operation, this call will return needed operations
	// and accumulate all operational errors encountered in the operation.
	// If the 'force' is not set, any error will be fatal and no operations will be returned.
	switch ops, err := op.destroyOps(); err {
	case errRefresh:
	case errAlreadyDying:
		return nil, jujutxn.ErrNoOperations
	case nil:
		return ops, nil
	default:
		if op.Force {
			logger.Warningf("force destroy remote application %v despite error %v", op.app, err)
			return ops, nil
		}
		return nil, err
	}
	return nil, jujutxn.ErrNoOperations
}

// Done is part of the ModelOperation interface.
func (op *DestroyRemoteApplicationOperation) Done(err error) error {
	// NOTE(tsm): if you change the business logic here, check
	//            that RemoveOfferOperation is modified to suit
	if err != nil {
		if !op.Force {
			return errors.Annotatef(err, "cannot destroy remote application %q", op.app)
		}
		op.AddError(errors.Errorf("force destroy of remote application %v failed but proceeded despite encountering ERROR %v", op.app, err))
	}
	return nil
}

// DestroyWithForce in addition to doing what Destroy() does,
// when force is passed in as 'true', forces th destruction of remote application,
// ignoring errors.
func (s *RemoteApplication) DestroyWithForce(force bool, maxWait time.Duration) (opErrs []error, err error) {
	defer func() {
		if err == nil {
			s.doc.Life = Dying
		}
	}()
	op := s.DestroyOperation(force)
	op.MaxWait = maxWait
	err = s.st.ApplyOperation(op)
	return op.Errors, err
}

// Destroy ensures that this remote application reference and all its relations
// will be removed at some point; if no relation involving the
// application has any units in scope, they are all removed immediately.
func (s *RemoteApplication) Destroy() error {
	errs, err := s.DestroyWithForce(false, time.Duration(0))
	if len(errs) != 0 {
		logger.Warningf("operational errors destroying remote application %v: %v", s.Name(), errs)
	}
	return err
}

// destroyOps returns the operations required to destroy the application. If it
// returns errRefresh, the application should be refreshed and the destruction
// operations recalculated.
// When 'force' is set, this call will return needed operations
// and accumulate all operational errors encountered in the operation.
// If the 'force' is not set, any error will be fatal and no operations will be returned.
func (op *DestroyRemoteApplicationOperation) destroyOps() (ops []txn.Op, err error) {
	if op.app.doc.Life == Dying {
		if !op.Force {
			return nil, errAlreadyDying
		}
	}
	haveRels := true
	rels, err := op.app.Relations()
	if err != nil {
		if !op.Force {
			return nil, errors.Trace(err)
		}
		op.AddError(err)
		haveRels = false
	}

	if haveRels && len(rels) != op.app.doc.RelationCount {
		// This is just an early bail out. The relations obtained may still
		// be wrong, but that situation will be caught by a combination of
		// asserts on relationcount and on each known relation, below.
		return nil, errRefresh
	}

	// We'll need status below when processing relations.
	statusInfo, statusErr := op.app.Status()
	if statusErr != nil && !errors.IsNotFound(statusErr) {
		if !op.Force {
			return nil, statusErr
		}
		op.AddError(statusErr)
	}

	removeCount := 0
	if haveRels {
		failRels := false
		for _, rel := range rels {
			// If the remote app has been terminated, we may have been offline
			// and not noticed so need to clean up any exiting relation units.
			if statusErr == nil && statusInfo.Status == status.Terminated {
				logger.Debugf("forcing cleanup of units for %v", op.app.Name())
				remoteUnits, err := rel.AllRemoteUnits(op.app.Name())
				if err != nil {
					op.AddError(err)
					failRels = true
					continue
				}
				if countRemoteUnits := len(remoteUnits); countRemoteUnits != 0 {
					logger.Debugf("got %v relation units to clean", countRemoteUnits)
					for _, ru := range remoteUnits {
						leaveScopeOps, err := ru.leaveScopeForcedOps(&op.ForcedOperation)
						if err != nil && err != jujutxn.ErrNoOperations {
							op.AddError(err)
							failRels = true
							continue
						}
						ops = append(ops, leaveScopeOps...)
					}
				}
			}
			// When 'force' is set, this call will return both needed operations
			// as well as all operational errors encountered.
			// If the 'force' is not set, any error will be fatal and no operations will be returned.
			relOps, isRemove, err := rel.destroyOps(op.app.doc.Name, &op.ForcedOperation)
			if err == errAlreadyDying {
				relOps = []txn.Op{{
					C:      relationsC,
					Id:     rel.doc.DocID,
					Assert: bson.D{{"life", Dying}},
				}}
			} else if err != nil {
				op.AddError(err)
				failRels = true
				continue
			}
			if isRemove {
				removeCount++
			}
			ops = append(ops, relOps...)
		}
		if !op.Force && failRels {
			return nil, errors.Trace(op.LastError())
		}
	}
	// If all of the application's known relations will be
	// removed, the application can also be removed.
	if op.app.doc.RelationCount == removeCount {
		hasLastRefs := bson.D{{"life", Alive}, {"relationcount", removeCount}}
		removeOps, err := op.app.removeOps(hasLastRefs)
		if err != nil {
			if !op.Force {
				return nil, errors.Trace(err)
			}
			op.AddError(err)
		}
		ops = append(ops, removeOps...)
		return ops, nil
	}
	// In all other cases, application removal will be handled as a consequence
	// of the removal of the relation referencing it. If any  relations have
	// been removed, they'll be caught by the operations collected above;
	// but if any has been added, we need to abort and add  a destroy op for
	// that relation too.
	// In combination, it's enough to check for count equality:
	// an add/remove will not touch the count, but  will be caught by
	// virtue of being a remove.
	notLastRefs := bson.D{
		{"life", Alive},
		{"relationcount", op.app.doc.RelationCount},
	}
	update := bson.D{{"$set", bson.D{{"life", Dying}}}}
	if removeCount != 0 {
		decref := bson.D{{"$inc", bson.D{{"relationcount", -removeCount}}}}
		update = append(update, decref...)
	}
	ops = append(ops, txn.Op{
		C:      remoteApplicationsC,
		Id:     op.app.doc.DocID,
		Assert: notLastRefs,
		Update: update,
	})
	return ops, nil
}

// removeOps returns the operations required to remove the application. Supplied
// asserts will be included in the operation on the application document.
func (s *RemoteApplication) removeOps(asserts bson.D) ([]txn.Op, error) {
	r := s.st.RemoteEntities()
	ops := []txn.Op{
		{
			C:      remoteApplicationsC,
			Id:     s.doc.DocID,
			Assert: asserts,
			Remove: true,
		},
		removeStatusOp(s.st, s.globalKey()),
	}
	tokenOps := r.removeRemoteEntityOps(s.Tag())
	ops = append(ops, tokenOps...)
	return ops, nil
}

// Status returns the status of the remote application.
func (s *RemoteApplication) Status() (status.StatusInfo, error) {
	return getStatus(s.st.db(), s.globalKey(), "remote application")
}

// SetStatus sets the status for the application.
func (s *RemoteApplication) SetStatus(info status.StatusInfo) error {
	if !info.Status.KnownWorkloadStatus() {
		return errors.Errorf("cannot set invalid status %q", info.Status)
	}

	return setStatus(s.st.db(), setStatusParams{
		badge:     "remote application",
		globalKey: s.globalKey(),
		status:    info.Status,
		message:   info.Message,
		rawData:   info.Data,
		updated:   timeOrNow(info.Since, s.st.clock()),
	})
}

// TerminateOperation returns a ModelOperation that will terminate this
// remote application when applied, ensuring that all units have left
// scope as well.
func (s *RemoteApplication) TerminateOperation(message string) ModelOperation {
	return &terminateRemoteApplicationOperation{
		app: s,
		doc: statusDoc{
			Status:     status.Terminated,
			StatusInfo: message,
			Updated:    s.st.clock().Now().UnixNano(),
		},
	}
}

type terminateRemoteApplicationOperation struct {
	app *RemoteApplication
	doc statusDoc
}

// Build is part of ModelOperation.
func (op *terminateRemoteApplicationOperation) Build(attempt int) ([]txn.Op, error) {
	ops, err := statusSetOps(op.app.st.db(), op.doc, op.app.globalKey())
	if err != nil {
		return nil, errors.Annotate(err, "setting status")
	}
	name := op.app.Name()
	logger.Debugf("leaving scope on all %q relation units", name)
	rels, err := op.app.Relations()
	if err != nil {
		return nil, errors.Annotatef(err, "getting relations for %q", name)
	}
	for _, rel := range rels {
		remoteUnits, err := rel.AllRemoteUnits(name)
		if err != nil {
			return nil, errors.Annotatef(err, "getting remote units for %q", name)
		}
		for _, ru := range remoteUnits {
			leaveOperation := ru.LeaveScopeOperation(false)
			leaveOps, err := leaveOperation.Build(attempt)
			if errors.Cause(err) == jujutxn.ErrNoOperations {
				continue
			} else if err != nil {
				return nil, errors.Annotatef(err, "leaving scope for %q", ru.key())
			}
			ops = append(ops, leaveOps...)
		}
	}
	return ops, nil
}

// Done is part of ModelOperation.
func (op *terminateRemoteApplicationOperation) Done(err error) error {
	if err != nil {
		return errors.Annotatef(err, "updating unit %q", op.app.Name())
	}
	probablyUpdateStatusHistory(op.app.st.db(), op.app.globalKey(), op.doc)
	return nil
}

// Endpoints returns the application's currently available relation endpoints.
func (s *RemoteApplication) Endpoints() ([]Endpoint, error) {
	return remoteEndpointDocsToEndpoints(s.Name(), s.doc.Endpoints), nil
}

func remoteEndpointDocsToEndpoints(applicationName string, docs []remoteEndpointDoc) []Endpoint {
	eps := make([]Endpoint, len(docs))
	for i, ep := range docs {
		eps[i] = Endpoint{
			ApplicationName: applicationName,
			Relation: charm.Relation{
				Name:      ep.Name,
				Role:      ep.Role,
				Interface: ep.Interface,
				Limit:     ep.Limit,
				Scope:     ep.Scope,
			}}
	}
	sort.Sort(epSlice(eps))
	return eps
}

// Endpoint returns the relation endpoint with the supplied name, if it exists.
func (s *RemoteApplication) Endpoint(relationName string) (Endpoint, error) {
	eps, err := s.Endpoints()
	if err != nil {
		return Endpoint{}, err
	}
	for _, ep := range eps {
		if ep.Name == relationName {
			return ep, nil
		}
	}
	return Endpoint{}, fmt.Errorf("remote application %q has no %q relation", s, relationName)
}

// AddEndpoints adds the specified endpoints to the remote application.
// If an endpoint with the same name already exists, an error is returned.
// If the endpoints change during the update, the operation is retried.
func (s *RemoteApplication) AddEndpoints(eps []charm.Relation) error {
	newEps := make([]remoteEndpointDoc, len(eps))
	for i, ep := range eps {
		newEps[i] = remoteEndpointDoc{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}

	model, err := s.st.Model()
	if err != nil {
		return errors.Trace(err)
	} else if model.Life() != Alive {
		return errors.Errorf("model is no longer alive")
	}

	checkCompatibleEndpoints := func(currentEndpoints []Endpoint) error {
		// Ensure there are no current endpoints with the same name as
		// any of those we want to update.
		currentEndpointNames := set.NewStrings()
		for _, ep := range currentEndpoints {
			currentEndpointNames.Add(ep.Name)
		}
		for _, r := range eps {
			if currentEndpointNames.Contains(r.Name) {
				return errors.AlreadyExistsf("endpoint %v", r.Name)
			}
		}
		return nil
	}

	currentEndpoints, err := s.Endpoints()
	if err != nil {
		return errors.Trace(err)
	}
	if err := checkCompatibleEndpoints(currentEndpoints); err != nil {
		return err
	}
	applicationID := s.st.docID(s.Name())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// model may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(s.st); err != nil {
				return nil, errors.Trace(err)
			}
			if err = s.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
			currentEndpoints, err = s.Endpoints()
			if err != nil {
				return nil, errors.Trace(err)
			}
			if err := checkCompatibleEndpoints(currentEndpoints); err != nil {
				return nil, err
			}
		}
		ops := []txn.Op{
			model.assertActiveOp(),
			{
				C:  remoteApplicationsC,
				Id: applicationID,
				Assert: bson.D{
					{"endpoints", bson.D{{
						"$not", bson.D{{
							"$elemMatch", bson.D{{
								"$in", newEps}},
						}},
					}}},
				},
				Update: bson.D{
					{"$addToSet", bson.D{{"endpoints", bson.D{{"$each", newEps}}}}},
				},
			},
		}
		return ops, nil
	}
	if err := s.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return s.Refresh()
}

func (s *RemoteApplication) Macaroon() (*macaroon.Macaroon, error) {
	if s.doc.Macaroon == "" {
		return nil, nil
	}
	var mac macaroon.Macaroon
	err := json.Unmarshal([]byte(s.doc.Macaroon), &mac)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &mac, nil
}

// String returns the application name.
func (s *RemoteApplication) String() string {
	return s.doc.Name
}

// Refresh refreshes the contents of the RemoteApplication from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// application has been removed.
func (s *RemoteApplication) Refresh() error {
	applications, closer := s.st.db().GetCollection(remoteApplicationsC)
	defer closer()

	err := applications.FindId(s.doc.DocID).One(&s.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("remote application %q", s)
	}
	if err != nil {
		return fmt.Errorf("cannot refresh application %q: %v", s, err)
	}
	return nil
}

// Relations returns a Relation for every relation the application is in.
func (s *RemoteApplication) Relations() (relations []*Relation, err error) {
	return applicationRelations(s.st, s.doc.Name)
}

// AddRemoteApplicationParams contains the parameters for adding a remote application
// to the model.
type AddRemoteApplicationParams struct {
	// Name is the name to give the remote application. This does not have to
	// match the application name in the URL, or the name in the remote model.
	Name string

	// OfferUUID is the UUID of the offer.
	OfferUUID string

	// URL is either empty, or the URL that the remote application was offered
	// with on the hosting model.
	URL string

	// SourceModel is the tag of the model to which the remote application belongs.
	SourceModel names.ModelTag

	// Token is an opaque string that identifies the remote application in the
	// source model.
	Token string

	// Endpoints describes the endpoints that the remote application implements.
	Endpoints []charm.Relation

	// Spaces describes the network spaces that the remote
	// application's endpoints inhabit in the remote model.
	Spaces []*environs.ProviderSpaceInfo

	// Bindings maps each endpoint name to the remote space it is bound to.
	Bindings map[string]string

	// IsConsumerProxy is true when a remote application is created as a result
	// of a registration operation from a remote model.
	IsConsumerProxy bool

	// Macaroon is used for authentication on the offering side.
	Macaroon *macaroon.Macaroon
}

// Validate returns an error if there's a problem with the
// parameters being used to create a remote application.
func (p AddRemoteApplicationParams) Validate() error {
	if !names.IsValidApplication(p.Name) {
		return errors.NotValidf("name %q", p.Name)
	}
	if p.URL != "" {
		// URL may be empty, to represent remote applications corresponding
		// to consumers of an offered application.
		if _, err := charm.ParseOfferURL(p.URL); err != nil {
			return errors.Annotate(err, "validating offer URL")
		}
	}
	if p.SourceModel == (names.ModelTag{}) {
		return errors.NotValidf("empty source model tag")
	}
	spaceNames := set.NewStrings()
	for _, space := range p.Spaces {
		spaceNames.Add(space.Name)
	}
	for endpoint, space := range p.Bindings {
		if !spaceNames.Contains(space) {
			return errors.NotValidf("endpoint %q bound to missing space %q", endpoint, space)
		}
	}
	return nil
}

// AddRemoteApplication creates a new remote application record, having the supplied relation endpoints,
// with the supplied name (which must be unique across all applications, local and remote).
func (st *State) AddRemoteApplication(args AddRemoteApplicationParams) (_ *RemoteApplication, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add remote application %q", args.Name)

	// Sanity checks.
	if err := args.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	} else if model.Life() != Alive {
		return nil, errors.Errorf("model is no longer alive")
	}

	var macJSON string
	if args.Macaroon != nil {
		b, err := json.Marshal(args.Macaroon)
		if err != nil {
			return nil, errors.Trace(err)
		}
		macJSON = string(b)
	}
	applicationID := st.docID(args.Name)
	// Create the application addition operations.
	appDoc := &remoteApplicationDoc{
		DocID:           applicationID,
		Name:            args.Name,
		OfferUUID:       args.OfferUUID,
		SourceModelUUID: args.SourceModel.Id(),
		URL:             args.URL,
		Bindings:        args.Bindings,
		Life:            Alive,
		IsConsumerProxy: args.IsConsumerProxy,
		Macaroon:        macJSON,
	}
	eps := make([]remoteEndpointDoc, len(args.Endpoints))
	for i, ep := range args.Endpoints {
		eps[i] = remoteEndpointDoc{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}
	appDoc.Endpoints = eps
	spaces := make([]remoteSpaceDoc, len(args.Spaces))
	for i, space := range args.Spaces {
		spaces[i] = remoteSpaceDoc{
			CloudType:          space.CloudType,
			Name:               space.Name,
			ProviderId:         string(space.ProviderId),
			ProviderAttributes: space.ProviderAttributes,
		}
		subnets := make([]remoteSubnetDoc, len(space.Subnets))
		for i, subnet := range space.Subnets {
			subnets[i] = remoteSubnetDoc{
				CIDR:              subnet.CIDR,
				ProviderId:        string(subnet.ProviderId),
				VLANTag:           subnet.VLANTag,
				AvailabilityZones: copyStrings(subnet.AvailabilityZones),
				ProviderSpaceId:   string(subnet.ProviderSpaceId),
				ProviderNetworkId: string(subnet.ProviderNetworkId),
			}
		}
		spaces[i].Subnets = subnets
	}
	appDoc.Spaces = spaces
	app := newRemoteApplication(st, appDoc)

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// model may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(st); err != nil {
				return nil, errors.Trace(err)
			}
			// Ensure a local application with the same name doesn't exist.
			if localExists, err := isNotDead(st, applicationsC, args.Name); err != nil {
				return nil, errors.Trace(err)
			} else if localExists {
				return nil, errors.AlreadyExistsf("local application with same name")
			}
			// Ensure a remote application with the same name doesn't exist.
			if exists, err := isNotDead(st, remoteApplicationsC, args.Name); err != nil {
				return nil, errors.Trace(err)
			} else if exists {
				return nil, errors.AlreadyExistsf("remote application")
			}
		}
		ops := []txn.Op{
			model.assertActiveOp(),
			{
				C:      remoteApplicationsC,
				Id:     appDoc.Name,
				Assert: txn.DocMissing,
				Insert: appDoc,
			}, {
				C:      applicationsC,
				Id:     appDoc.Name,
				Assert: txn.DocMissing,
			},
		}
		if !args.IsConsumerProxy {
			statusDoc := statusDoc{
				ModelUUID: st.ModelUUID(),
				Status:    status.Unknown,
				Updated:   st.clock().Now().UnixNano(),
			}
			ops = append(ops, createStatusOp(st, app.globalKey(), statusDoc))
		}
		// If we know the token, import it.
		if args.Token != "" {
			importRemoteEntityOps := st.RemoteEntities().importRemoteEntityOps(app.Tag(), args.Token)
			ops = append(ops, importRemoteEntityOps...)
		}
		return ops, nil
	}
	if err = st.db().Run(buildTxn); err != nil {
		return nil, errors.Trace(err)
	}
	return app, nil
}

// RemoteApplication returns a remote application state by name.
func (st *State) RemoteApplication(name string) (_ *RemoteApplication, err error) {
	if !names.IsValidApplication(name) {
		return nil, errors.NotValidf("remote application name %q", name)
	}

	applications, closer := st.db().GetCollection(remoteApplicationsC)
	defer closer()

	appDoc := &remoteApplicationDoc{}
	err = applications.FindId(name).One(appDoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("remote application %q", name)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get remote application %q", name)
	}
	return newRemoteApplication(st, appDoc), nil
}

// RemoteApplicationByToken returns a remote application state by token.
func (st *State) RemoteApplicationByToken(token string) (_ *RemoteApplication, err error) {
	apps, closer := st.db().GetCollection(remoteApplicationsC)
	defer closer()

	appDoc := &remoteApplicationDoc{}
	err = apps.Find(bson.D{{"token", token}}).One(appDoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("remote application with token %q", token)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get remote application with token %q", token)
	}
	return newRemoteApplication(st, appDoc), nil
}

// AllRemoteApplications returns all the remote applications used by the model.
func (st *State) AllRemoteApplications() (applications []*RemoteApplication, err error) {
	applicationsCollection, closer := st.db().GetCollection(remoteApplicationsC)
	defer closer()

	appDocs := []remoteApplicationDoc{}
	err = applicationsCollection.Find(bson.D{}).All(&appDocs)
	if err != nil {
		return nil, errors.Errorf("cannot get all remote applications")
	}
	for _, v := range appDocs {
		applications = append(applications, newRemoteApplication(st, &v))
	}
	return applications, nil
}
