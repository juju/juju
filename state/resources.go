// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v3"
	"github.com/kr/pretty"

	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/state/storage"
)

// Resources describes the state functionality for resources.
type Resources interface {
	// ListResources returns the list of resources for the given application.
	ListResources(applicationID string) (resources.ApplicationResources, error)

	// ListPendingResources returns the list of pending resources for
	// the given application.
	ListPendingResources(applicationID string) (resources.ApplicationResources, error)

	// AddPendingResource adds the resource to the data store in a
	// "pending" state. It will stay pending (and unavailable) until
	// it is resolved. The returned ID is used to identify the pending
	// resources when resolving it.
	AddPendingResource(applicationID, userID string, chRes charmresource.Resource) (string, error)

	// GetResource returns the identified resource.
	GetResource(applicationID, name string) (resources.Resource, error)

	// GetPendingResource returns the identified resource.
	GetPendingResource(applicationID, name, pendingID string) (resources.Resource, error)

	// SetResource adds the resource to blob storage and updates the metadata.
	SetResource(applicationID, userID string, res charmresource.Resource, r io.Reader, _ IncrementCharmModifiedVersionType) (resources.Resource, error)

	// SetUnitResource sets the resource metadata for a specific unit.
	SetUnitResource(unitName, userID string, res charmresource.Resource) (resources.Resource, error)

	// UpdatePendingResource adds the resource to blob storage and updates the metadata.
	UpdatePendingResource(applicationID, pendingID, userID string, res charmresource.Resource, r io.Reader) (resources.Resource, error)

	// OpenResource returns the metadata for a resource and a reader for the resource.
	OpenResource(applicationID, name string) (resources.Resource, io.ReadCloser, error)

	// OpenResourceForUniter returns the metadata for a resource and a reader for the resource.
	OpenResourceForUniter(unitName, name string) (resources.Resource, io.ReadCloser, error)

	// SetCharmStoreResources sets the "polled" resources for the
	// application to the provided values.
	SetCharmStoreResources(applicationID string, info []charmresource.Resource, lastPolled time.Time) error

	// RemovePendingAppResources removes any pending application-level
	// resources for the named application. This is used to clean up
	// resources for a failed application deployment.
	RemovePendingAppResources(applicationID string, pendingIDs map[string]string) error
}

// Resources returns the resources functionality for the current state.
func (st *State) Resources() Resources {
	return st.resources()
}

// Resources returns the resources functionality for the current state.
func (st *State) resources() *resourcePersistence {
	return &resourcePersistence{
		st:                    st,
		storage:               storage.NewStorage(st.ModelUUID(), st.MongoSession()),
		dockerMetadataStorage: NewDockerMetadataStorage(st),
	}
}

var rLogger = logger.Child("resource")

const (
	resourcesStagedIDSuffix     = "#staged"
	resourcesCharmstoreIDSuffix = "#charmstore"
)

// A change in CharmModifiedVersion triggers the uniter to run the upgrade_charm
// hook (and config hook). Increment required for a running unit to pick up
// new resources from `attach` or when a charm is upgraded without a new charm
// revision.
//
// IncrementCharmModifiedVersionType is the argument type for incrementing CharmModifiedVersion or not.
type IncrementCharmModifiedVersionType bool

const (
	// IncrementCharmModifiedVersion means CharmModifiedVersion needs to be incremented.
	IncrementCharmModifiedVersion IncrementCharmModifiedVersionType = true
	// DoNotIncrementCharmModifiedVersion means CharmModifiedVersion should not be incremented.
	DoNotIncrementCharmModifiedVersion IncrementCharmModifiedVersionType = false
)

// resourceID converts an external resource ID into an internal one.
func resourceID(id, subType, subID string) string {
	if subType == "" {
		return fmt.Sprintf("resource#%s", id)
	}
	return fmt.Sprintf("resource#%s#%s-%s", id, subType, subID)
}

func applicationResourceID(id string) string {
	return resourceID(id, "", "")
}

func pendingResourceID(id, pendingID string) string {
	return resourceID(id, "pending", pendingID)
}

func charmStoreResourceID(id string) string {
	return applicationResourceID(id) + resourcesCharmstoreIDSuffix
}

func unitResourceID(id, unitID string) string {
	return resourceID(id, "unit", unitID)
}

// stagedResourceID converts an external resource ID into an internal
// staged one.
func stagedResourceID(id string) string {
	return applicationResourceID(id) + resourcesStagedIDSuffix
}

// newPendingID generates a new unique identifier for a resource.
func newPendingID() (string, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", errors.Annotate(err, "could not create new resource ID")
	}
	return uuid.String(), nil
}

// newAppResourceID produces a new ID to use for the resource in the model.
func newAppResourceID(applicationID, name string) string {
	return fmt.Sprintf("%s/%s", applicationID, name)
}

// resourceDoc is the top-level document for resources.
type resourceDoc struct {
	DocID     string `bson:"_id"`
	ID        string `bson:"resource-id"`
	PendingID string `bson:"pending-id"`

	ApplicationID string `bson:"application-id"`
	UnitID        string `bson:"unit-id"`

	Name        string `bson:"name"`
	Type        string `bson:"type"`
	Path        string `bson:"path"`
	Description string `bson:"description"`

	Origin      string `bson:"origin"`
	Revision    int    `bson:"revision"`
	Fingerprint []byte `bson:"fingerprint"`
	Size        int64  `bson:"size"`

	Username  string    `bson:"username"`
	Timestamp time.Time `bson:"timestamp-when-added"`

	StoragePath string `bson:"storage-path"`

	DownloadProgress *int64 `bson:"download-progress,omitempty"`

	LastPolled time.Time `bson:"timestamp-when-last-polled"`
}

// storedResource holds all model-stored information for a resource.
type storedResource struct {
	resources.Resource

	// storagePath is the path to where the resource content is stored.
	storagePath string
}

// charmStoreResource holds the info for a resource as provided by the
// charm store at as specific point in time.
type charmStoreResource struct {
	charmresource.Resource
	id            string
	applicationID string
	lastPolled    time.Time
}

func charmStoreResource2Doc(id string, res charmStoreResource) *resourceDoc {
	stored := storedResource{
		Resource: resources.Resource{
			Resource:      res.Resource,
			ID:            res.id,
			ApplicationID: res.applicationID,
		},
	}
	doc := resource2doc(id, stored)
	doc.LastPolled = res.lastPolled
	return doc
}

// newCharmStoreResourceDoc generates a doc that represents the given resource.
func newCharmStoreResourceDoc(res charmStoreResource) *resourceDoc {
	fullID := charmStoreResourceID(res.id)
	return charmStoreResource2Doc(fullID, res)
}

// newUnitResourceDoc generates a doc that represents the given resource.
func newUnitResourceDoc(unitID string, stored storedResource) *resourceDoc {
	fullID := unitResourceID(stored.ID, unitID)
	return unitResource2Doc(fullID, unitID, stored)
}

// newResourceDoc generates a doc that represents the given resource.
func newResourceDoc(stored storedResource) *resourceDoc {
	fullID := applicationResourceID(stored.ID)
	if stored.PendingID != "" {
		fullID = pendingResourceID(stored.ID, stored.PendingID)
	}
	return resource2doc(fullID, stored)
}

// newStagedResourceDoc generates a staging doc that represents
// the given resource.
func newStagedResourceDoc(stored storedResource) *resourceDoc {
	stagedID := stagedResourceID(stored.ID)
	return resource2doc(stagedID, stored)
}

func unitResource2Doc(id, unitID string, stored storedResource) *resourceDoc {
	doc := resource2doc(id, stored)
	doc.UnitID = unitID
	return doc
}

// resource2doc converts the resource into a DB doc.
func resource2doc(id string, stored storedResource) *resourceDoc {
	res := stored.Resource
	// TODO(ericsnow) We may need to limit the resolution of timestamps
	// in order to avoid some conversion problems from Mongo.
	return &resourceDoc{
		DocID:     id,
		ID:        res.ID,
		PendingID: res.PendingID,

		ApplicationID: res.ApplicationID,

		Name:        res.Name,
		Type:        res.Type.String(),
		Path:        res.Path,
		Description: res.Description,

		Origin:      res.Origin.String(),
		Revision:    res.Revision,
		Fingerprint: res.Fingerprint.Bytes(),
		Size:        res.Size,

		Username:  res.Username,
		Timestamp: res.Timestamp,

		StoragePath: stored.storagePath,
	}
}

// doc2resource returns the resource info represented by the doc.
func doc2resource(doc resourceDoc) (storedResource, error) {
	res, err := doc2basicResource(doc)
	if err != nil {
		return storedResource{}, errors.Trace(err)
	}

	stored := storedResource{
		Resource:    res,
		storagePath: doc.StoragePath,
	}
	return stored, nil
}

// doc2basicResource returns the resource info represented by the doc.
func doc2basicResource(doc resourceDoc) (resources.Resource, error) {
	var res resources.Resource

	resType, err := charmresource.ParseType(doc.Type)
	if err != nil {
		return res, errors.Annotate(err, "got invalid data from DB")
	}

	origin, err := charmresource.ParseOrigin(doc.Origin)
	if err != nil {
		return res, errors.Annotate(err, "got invalid data from DB")
	}

	fp, err := resources.DeserializeFingerprint(doc.Fingerprint)
	if err != nil {
		return res, errors.Annotate(err, "got invalid data from DB")
	}

	res = resources.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        doc.Name,
				Type:        resType,
				Path:        doc.Path,
				Description: doc.Description,
			},
			Origin:      origin,
			Revision:    doc.Revision,
			Fingerprint: fp,
			Size:        doc.Size,
		},
		ID:            doc.ID,
		PendingID:     doc.PendingID,
		ApplicationID: doc.ApplicationID,
		Username:      doc.Username,
		Timestamp:     doc.Timestamp,
	}
	if err := res.Validate(); err != nil {
		return res, errors.Annotate(err, "got invalid data from DB")
	}
	return res, nil
}

// storagePath returns the path used as the location where the resource
// is stored in state storage. This requires that the returned string
// be unique and that it be organized in a structured way. In this case
// we start with a top-level (the application), then under that application use
// the "resources" section. The provided ID is located under there.
func storagePath(name, applicationID, pendingID string) string {
	// TODO(ericsnow) Use applications/<application>/resources/<resource>?
	id := name
	if pendingID != "" {
		// TODO(ericsnow) How to resolve this later?
		id += "-" + pendingID
	}
	return path.Join("application-"+applicationID, "resources", id)
}

// resourcePersistence provides the persistence
// functionality for resources.
type resourcePersistence struct {
	st                    *State
	storage               storage.Storage
	dockerMetadataStorage DockerMetadataStorage
}

// One gets the identified document from the collection.
func (sp *resourcePersistence) one(collName, id string, doc interface{}) error {
	coll, closeColl := sp.st.db().GetCollection(collName)
	defer closeColl()

	err := coll.FindId(id).One(doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf(id)
	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// All gets all documents from the collection matching the query.
func (p *resourcePersistence) all(collName string, query, docs interface{}) error {
	coll, closeColl := p.st.db().GetCollection(collName)
	defer closeColl()

	if err := coll.Find(query).All(docs); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// resources returns the resource docs for the given application.
func (p *resourcePersistence) resources(applicationID string) ([]resourceDoc, error) {
	rLogger.Tracef("querying db for resources for %q", applicationID)
	var docs []resourceDoc
	query := bson.D{{"application-id", applicationID}}
	if err := p.all(resourcesC, query, &docs); err != nil {
		return nil, errors.Trace(err)
	}
	rLogger.Tracef("found %d resources", len(docs))
	return docs, nil
}

func (p *resourcePersistence) unitResources(unitID string) ([]resourceDoc, error) {
	var docs []resourceDoc
	query := bson.D{{"unit-id", unitID}}
	if err := p.all(resourcesC, query, &docs); err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

// getOne returns the resource that matches the provided model ID.
func (p *resourcePersistence) getOne(resID string) (resourceDoc, error) {
	id := applicationResourceID(resID)
	rLogger.Tracef("querying db for resource %q as %q", resID, id)
	var doc resourceDoc
	if err := p.one(resourcesC, id, &doc); err != nil {
		return doc, errors.Trace(err)
	}
	return doc, nil
}

// getOnePending returns the resource that matches the provided model ID.
func (p *resourcePersistence) getOnePending(resID, pendingID string) (resourceDoc, error) {
	id := pendingResourceID(resID, pendingID)
	rLogger.Tracef("querying db for resource %q (pending %q) as %q", resID, pendingID, id)
	var doc resourceDoc
	if err := p.one(resourcesC, id, &doc); err != nil {
		return doc, errors.Trace(err)
	}
	return doc, nil
}

func (p *resourcePersistence) verifyApplication(id string) error {
	app, err := p.st.Application(id)
	if err != nil {
		return errors.Trace(err)
	}
	if app.Life() != Alive {
		return errors.NewNotFound(nil, fmt.Sprintf("application %q dying or dead", id))
	}
	return nil
}

// listResources returns the info for each non-pending resource of the
// identified application.
func (p *resourcePersistence) listResources(applicationID string, pending bool) (resources.ApplicationResources, error) {
	rLogger.Tracef("listing all resources for application %q, pending=%v", applicationID, pending)

	docs, err := p.resources(applicationID)
	if err != nil {
		return resources.ApplicationResources{}, errors.Trace(err)
	}

	store := map[string]charmresource.Resource{}
	units := map[names.UnitTag][]resources.Resource{}
	downloadProgress := make(map[names.UnitTag]map[string]int64)

	var results resources.ApplicationResources
	for _, doc := range docs {
		if !pending && doc.PendingID != "" {
			continue
		}
		if pending && doc.PendingID == "" {
			continue
		}

		res, err := doc2basicResource(doc)
		if err != nil {
			return resources.ApplicationResources{}, errors.Trace(err)
		}
		if strings.HasSuffix(doc.DocID, resourcesCharmstoreIDSuffix) {
			store[res.Name] = res.Resource
			continue
		}
		if doc.UnitID == "" {
			results.Resources = append(results.Resources, res)
			continue
		}
		tag := names.NewUnitTag(doc.UnitID)
		units[tag] = append(units[tag], res)
		if doc.DownloadProgress != nil {
			if downloadProgress[tag] == nil {
				downloadProgress[tag] = make(map[string]int64)
			}
			downloadProgress[tag][doc.Name] = *doc.DownloadProgress
		}
	}
	for _, res := range results.Resources {
		storeRes, ok := store[res.Name]
		if ok {
			results.CharmStoreResources = append(results.CharmStoreResources, storeRes)
		}
	}
	for tag, res := range units {
		results.UnitResources = append(results.UnitResources, resources.UnitResources{
			Tag:              tag,
			Resources:        res,
			DownloadProgress: downloadProgress[tag],
		})
	}
	if rLogger.IsTraceEnabled() {
		rLogger.Tracef("found %d docs: %q", len(docs), pretty.Sprint(results))
	}
	return results, nil
}

// ListPendingResources returns the extended, model-related info for
// each pending resource of the identifies application.
func (p *resourcePersistence) ListPendingResources(applicationID string) (resources.ApplicationResources, error) {
	rLogger.Tracef("listing all pending resources for application %q", applicationID)
	res, err := p.listResources(applicationID, true)
	if err != nil {
		if err := p.verifyApplication(applicationID); err != nil {
			return resources.ApplicationResources{}, errors.Trace(err)
		}
		return resources.ApplicationResources{}, errors.Trace(err)
	}
	return res, nil
}

// ListResources returns the non pending resource data for the given application ID.
func (p *resourcePersistence) ListResources(applicationID string) (resources.ApplicationResources, error) {
	res, err := p.listResources(applicationID, false)
	if err != nil {
		if err := p.verifyApplication(applicationID); err != nil {
			return resources.ApplicationResources{}, errors.Trace(err)
		}
		return resources.ApplicationResources{}, errors.Trace(err)
	}

	units, err := allUnits(p.st, applicationID)
	if err != nil {
		return resources.ApplicationResources{}, errors.Trace(err)
	}
	for _, u := range units {
		found := false
		for _, unitRes := range res.UnitResources {
			if u.Tag().String() == unitRes.Tag.String() {
				found = true
				break
			}
		}
		if !found {
			unitRes := resources.UnitResources{
				Tag: u.UnitTag(),
			}
			res.UnitResources = append(res.UnitResources, unitRes)
		}
	}

	return res, nil
}

// GetResource returns the resource data for the identified resource.
func (p *resourcePersistence) GetResource(applicationID, name string) (resources.Resource, error) {
	id := newAppResourceID(applicationID, name)
	res, _, err := p.getResource(id)
	if err != nil {
		if err := p.verifyApplication(applicationID); err != nil {
			return resources.Resource{}, errors.Trace(err)
		}
		return res, errors.Trace(err)
	}
	return res, nil
}

// GetPendingResource returns the resource data for the identified resource.
func (p *resourcePersistence) GetPendingResource(applicationID, name, pendingID string) (resources.Resource, error) {
	var res resources.Resource

	resources, err := p.ListPendingResources(applicationID)
	if err != nil {
		// We do not call VerifyApplication() here because pending resources
		// do not have to have an existing application.
		return res, errors.Trace(err)
	}

	for _, res := range resources.Resources {
		if res.Name == name && res.PendingID == pendingID {
			return res, nil
		}
	}
	return res, errors.NotFoundf("pending resource %q (%s)", name, pendingID)
}

// getResource returns the extended, model-related info for the non-pending
// resource.
func (p *resourcePersistence) getResource(id string) (res resources.Resource, storagePath string, _ error) {
	rLogger.Tracef("get resource %q", id)
	doc, err := p.getOne(id)
	if err != nil {
		return res, "", errors.Trace(err)
	}

	stored, err := doc2resource(doc)
	if err != nil {
		return res, "", errors.Trace(err)
	}

	return stored.Resource, stored.storagePath, nil
}

// SetResource stores the resource in the Juju model.
func (p *resourcePersistence) SetResource(
	applicationID, userID string,
	chRes charmresource.Resource,
	r io.Reader, incrementCharmModifiedVersion IncrementCharmModifiedVersionType,
) (resources.Resource, error) {
	rLogger.Tracef("adding resource %q for application %q", chRes.Name, applicationID)
	pendingID := ""
	res, err := p.setResource(pendingID, applicationID, userID, chRes, r, incrementCharmModifiedVersion)
	if err != nil {
		return res, errors.Trace(err)
	}
	return res, nil
}

// SetUnitResource stores the resource info for a particular unit. The
// resource must already be set for the application.
func (p *resourcePersistence) SetUnitResource(unitName, userID string, chRes charmresource.Resource) (_ resources.Resource, outErr error) {
	rLogger.Tracef("adding resource %q for unit %q", chRes.Name, unitName)
	var empty resources.Resource

	applicationID, err := names.UnitApplication(unitName)
	if err != nil {
		return empty, errors.Trace(err)
	}

	res := resources.Resource{
		Resource:      chRes,
		ID:            newAppResourceID(applicationID, chRes.Name),
		ApplicationID: applicationID,
	}
	res.Username = userID
	res.Timestamp = p.st.clock().Now().UTC()
	if err := res.Validate(); err != nil {
		return empty, errors.Annotate(err, "bad resource metadata")
	}

	if err := p.setUnitResourceProgress(unitName, res, nil); err != nil {
		return empty, errors.Trace(err)
	}

	return res, nil
}

// SetCharmStoreResources sets the "polled" resources for the
// application to the provided values.
func (p *resourcePersistence) SetCharmStoreResources(applicationID string, info []charmresource.Resource, lastPolled time.Time) error {
	for _, chRes := range info {
		id := newAppResourceID(applicationID, chRes.Name)
		if err := p.setCharmStoreResource(id, applicationID, chRes, lastPolled); err != nil {
			return errors.Trace(err)
		}
		// TODO(ericsnow) Worry about extras? missing?
	}

	return nil
}

// setCharmStoreResource stores the resource info that was retrieved
// from the charm store.
func (p *resourcePersistence) setCharmStoreResource(id, applicationID string, res charmresource.Resource, lastPolled time.Time) error {
	rLogger.Tracef("set charmstore %q resource %q", applicationID, res.Name)
	if err := res.Validate(); err != nil {
		return errors.Annotate(err, "bad resource")
	}
	if lastPolled.IsZero() {
		return errors.NotValidf("empty last polled timestamp for charm resource %s/%s", applicationID, id)
	}

	csRes := charmStoreResource{
		Resource:      res,
		id:            id,
		applicationID: applicationID,
		lastPolled:    lastPolled,
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// This is an "upsert".
		var ops []txn.Op
		switch attempt {
		case 0:
			ops = newInsertCharmStoreResourceOps(csRes)
		case 1:
			ops = newUpdateCharmStoreResourceOps(csRes)
		default:
			// Either insert or update will work so we should not get here.
			return nil, errors.New("setting the resource failed")
		}
		// No pending resources so we always do this here.
		ops = append(ops, applicationExistsOps(applicationID)...)
		return ops, nil
	}
	if err := p.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// AddPendingResource stores the resource in the Juju model.
func (st resourcePersistence) AddPendingResource(applicationID, userID string, chRes charmresource.Resource) (pendingID string, err error) {
	pendingID, err = newPendingID()
	if err != nil {
		return "", errors.Annotate(err, "could not generate resource ID")
	}
	rLogger.Debugf("adding pending resource %q for application %q (ID: %s)", chRes.Name, applicationID, pendingID)

	if _, err := st.setResource(pendingID, applicationID, userID, chRes, nil, IncrementCharmModifiedVersion); err != nil {
		return "", errors.Trace(err)
	}

	return pendingID, nil
}

// UpdatePendingResource stores the resource in the Juju model.
func (st resourcePersistence) UpdatePendingResource(applicationID, pendingID, userID string, chRes charmresource.Resource, r io.Reader) (resources.Resource, error) {
	rLogger.Tracef("updating pending resource %q (%s) for application %q", chRes.Name, pendingID, applicationID)
	res, err := st.setResource(pendingID, applicationID, userID, chRes, r, IncrementCharmModifiedVersion)
	if err != nil {
		return res, errors.Trace(err)
	}
	return res, nil
}

// OpenResource returns metadata about the resource, and a reader for
// the resource.
func (p *resourcePersistence) OpenResource(applicationID, name string) (resources.Resource, io.ReadCloser, error) {
	rLogger.Tracef("open resource %q of %q", name, applicationID)
	id := newAppResourceID(applicationID, name)
	resourceInfo, storagePath, err := p.getResource(id)
	if err != nil {
		if err := p.verifyApplication(applicationID); err != nil {
			return resources.Resource{}, nil, errors.Trace(err)
		}
		return resources.Resource{}, nil, errors.Annotate(err, "while getting resource info")
	}
	if resourceInfo.IsPlaceholder() {
		rLogger.Tracef("placeholder resource %q treated as not found", name)
		return resources.Resource{}, nil, errors.NotFoundf("resource %q", name)
	}

	var resourceReader io.ReadCloser
	var resSize int64
	switch resourceInfo.Type {
	case charmresource.TypeContainerImage:
		resourceReader, resSize, err = p.dockerMetadataStorage.Get(resourceInfo.ID)
	case charmresource.TypeFile:
		resourceReader, resSize, err = p.storage.Get(storagePath)
	default:
		return resources.Resource{}, nil, errors.New("unknown resource type")
	}
	if err != nil {
		return resources.Resource{}, nil, errors.Annotate(err, "while retrieving resource data")
	}
	switch resourceInfo.Type {
	case charmresource.TypeContainerImage:
		// Resource size only found at this stage in time as it's a response from the charmstore, not a stored file.
		// Store it as it's used later for verification (in a separate call than this one)
		resourceInfo.Size = resSize
		if err := p.storeResourceInfo(resourceInfo); err != nil {
			return resources.Resource{}, nil, errors.Annotate(err, "failed to update resource details with docker detail size")
		}
	case charmresource.TypeFile:
		if resSize != resourceInfo.Size {
			msg := "storage returned a size (%d) which doesn't match resource metadata (%d)"
			return resources.Resource{}, nil, errors.Errorf(msg, resSize, resourceInfo.Size)
		}
	}

	return resourceInfo, resourceReader, nil
}

// OpenResourceForUniter returns metadata about the resource and
// a reader for the resource. The resource is associated with
// the unit once the reader is completely exhausted.
func (p *resourcePersistence) OpenResourceForUniter(unitName, resName string) (resources.Resource, io.ReadCloser, error) {
	rLogger.Tracef("open resource %q for uniter %q", resName, unitName)

	pendingID, err := newPendingID()
	if err != nil {
		return resources.Resource{}, nil, errors.Trace(err)
	}

	appName, err := names.UnitApplication(unitName)
	if err != nil {
		return resources.Resource{}, nil, errors.Trace(err)
	}
	resourceInfo, resourceReader, err := p.OpenResource(appName, resName)
	if err != nil {
		return resources.Resource{}, nil, errors.Trace(err)
	}

	pending := resourceInfo // a copy
	pending.PendingID = pendingID

	progress := int64(0)
	if err := p.setUnitResourceProgress(unitName, pending, &progress); err != nil {
		_ = resourceReader.Close()
		return resources.Resource{}, nil, errors.Trace(err)
	}

	resourceReader = &unitSetter{
		ReadCloser: resourceReader,
		persist:    p,
		unitName:   unitName,
		pending:    pending,
		resource:   resourceInfo,
		clock:      clock.WallClock,
	}

	return resourceInfo, resourceReader, nil
}

// unitSetter records the resource as in use by a unit when the wrapped
// reader has been fully read.
type unitSetter struct {
	io.ReadCloser
	persist            *resourcePersistence
	unitName           string
	pending            resources.Resource
	resource           resources.Resource
	progress           int64
	lastProgressUpdate time.Time
	clock              clock.Clock
}

// Read implements io.Reader.
func (u *unitSetter) Read(p []byte) (n int, err error) {
	n, err = u.ReadCloser.Read(p)
	if err == io.EOF {
		// record that the unit is now using this version of the resource
		if err := u.persist.setUnitResourceProgress(u.unitName, u.resource, nil); err != nil {
			msg := "Failed to record that unit %q is using resource %q revision %v"
			rLogger.Errorf(msg, u.unitName, u.resource.Name, u.resource.RevisionString())
		}
	} else {
		u.progress += int64(n)
		if time.Since(u.lastProgressUpdate) > time.Second {
			u.lastProgressUpdate = u.clock.Now()
			if err := u.persist.setUnitResourceProgress(u.unitName, u.pending, &u.progress); err != nil {
				rLogger.Errorf("failed to track progress: %v", err)
			}
		}
	}
	return n, err
}

// stageResource adds the resource in a separate staging area
// if the resource isn't already staged. If it is then
// errors.AlreadyExists is returned. A wrapper around the staged
// resource is returned which supports both finalizing and removing
// the staged resource.
func (p *resourcePersistence) stageResource(res resources.Resource, storagePath string) (*StagedResource, error) {
	rLogger.Tracef("stage resource %q for %q", res.Name, res.ApplicationID)
	if storagePath == "" {
		return nil, errors.Errorf("missing storage path")
	}

	if err := res.Validate(); err != nil {
		return nil, errors.Annotate(err, "bad resource")
	}

	stored := storedResource{
		Resource:    res,
		storagePath: storagePath,
	}
	staged := &StagedResource{
		p:      p,
		id:     res.ID,
		stored: stored,
	}
	if err := staged.stage(); err != nil {
		return nil, errors.Trace(err)
	}
	return staged, nil
}

func (p *resourcePersistence) setResource(
	pendingID, applicationID, userID string,
	chRes charmresource.Resource, r io.Reader,
	incrementCharmModifiedVersion IncrementCharmModifiedVersionType,
) (resources.Resource, error) {
	id := newAppResourceID(applicationID, chRes.Name)

	res := resources.Resource{
		Resource:      chRes,
		ID:            id,
		PendingID:     pendingID,
		ApplicationID: applicationID,
	}
	if r != nil {
		// TODO(ericsnow) Validate the user ID (or use a tag).
		res.Username = userID
		res.Timestamp = p.st.clock().Now().UTC()
	}

	if err := res.Validate(); err != nil {
		return res, errors.Annotate(err, "bad resource metadata")
	}

	if r == nil {
		if err := p.storeResourceInfo(res); err != nil {
			return res, errors.Trace(err)
		}
	} else {
		if err := p.storeResource(res, r, incrementCharmModifiedVersion); err != nil {
			return res, errors.Trace(err)
		}
	}

	return res, nil
}

func (p *resourcePersistence) getStored(res resources.Resource) (storedResource, error) {
	doc, err := p.getOne(res.ID)
	if errors.IsNotFound(err) {
		err = errors.NotFoundf("resource %q", res.Name)
	}
	if err != nil {
		return storedResource{}, errors.Trace(err)
	}

	stored, err := doc2resource(doc)
	if err != nil {
		return stored, errors.Trace(err)
	}

	return stored, nil
}

// storeResource stores the info for the resource.
func (p *resourcePersistence) storeResourceInfo(res resources.Resource) error {
	rLogger.Tracef("set resource %q for %q", res.Name, res.ApplicationID)
	stored, err := p.getStored(res)
	if errors.IsNotFound(err) {
		stored = storedResource{Resource: res}
	} else if err != nil {
		return errors.Trace(err)
	}
	// TODO(ericsnow) Ensure that stored.Resource matches res? If we do
	// so then the following line is unnecessary.
	stored.Resource = res

	if err := res.Validate(); err != nil {
		return errors.Annotate(err, "bad resource")
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// This is an "upsert".
		var ops []txn.Op
		switch attempt {
		case 0:
			ops = newInsertResourceOps(stored)
		case 1:
			ops = newUpdateResourceOps(stored)
		default:
			// Either insert or update will work so we should not get here.
			return nil, errors.New("setting the resource failed")
		}
		if stored.PendingID == "" {
			// Only non-pending resources must have an existing application.
			ops = append(ops, applicationExistsOps(res.ApplicationID)...)
		}
		return ops, nil
	}
	if err := p.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (p *resourcePersistence) storeResource(res resources.Resource, r io.Reader, incrementCharmModifiedVersion IncrementCharmModifiedVersionType) (err error) {
	// We use a staging approach for adding the resource metadata
	// to the model. This is necessary because the resource data
	// is stored separately and adding to both should be an atomic
	// operation.

	storagePath := storagePath(res.Name, res.ApplicationID, res.PendingID)
	staged, err := p.stageResource(res, storagePath)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		if err != nil {
			if e := staged.Unstage(); e != nil {
				rLogger.Errorf("could not unstage resource %q (application %q): %v", res.Name, res.ApplicationID, e)
			}
		}
	}()

	hash := res.Fingerprint.String()
	switch res.Type {
	case charmresource.TypeFile:
		if err = p.storage.PutAndCheckHash(storagePath, r, res.Size, hash); err != nil {
			return errors.Trace(err)
		}
	case charmresource.TypeContainerImage:
		respBuf := new(bytes.Buffer)
		_, err = respBuf.ReadFrom(r)
		if err != nil {
			return errors.Trace(err)
		}
		dockerDetails, err := resources.UnmarshalDockerResource(respBuf.Bytes())
		if err != nil {
			return errors.Trace(err)
		}
		err = p.dockerMetadataStorage.Save(res.ID, dockerDetails)
		if err != nil {
			return errors.Trace(err)
		}
	}

	if err = staged.Activate(incrementCharmModifiedVersion); err != nil {
		if e := p.storage.Remove(storagePath); e != nil {
			rLogger.Errorf("could not remove resource %q (application %q) from storage: %v", res.Name, res.ApplicationID, e)
		}
		return errors.Trace(err)
	}
	return nil
}

// setUnitResourceProgress stores the resource info for a particular unit. The
// resource must already be set for the application. The provided progress
// is stored in the DB.
func (p *resourcePersistence) setUnitResourceProgress(unitID string, res resources.Resource, progress *int64) error {
	rLogger.Tracef("set unit %q resource %q progress", unitID, res.Name)
	if res.PendingID == "" && progress != nil {
		return errors.Errorf("only pending resources may track progress")
	}
	stored, err := p.getStored(res)
	if err != nil {
		return errors.Trace(err)
	}
	// TODO(ericsnow) Ensure that stored.Resource matches res? If we do
	// so then the following line is unnecessary.
	stored.Resource = res

	if err := res.Validate(); err != nil {
		return errors.Annotate(err, "bad resource")
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// This is an "upsert".
		var ops []txn.Op
		switch attempt {
		case 0:
			ops = newInsertUnitResourceOps(unitID, stored, progress)
		case 1:
			ops = newUpdateUnitResourceOps(unitID, stored, progress)
		default:
			// Either insert or update will work so we should not get here.
			return nil, errors.New("setting the resource failed")
		}
		// No pending resources so we always do this here.
		ops = append(ops, applicationExistsOps(res.ApplicationID)...)
		return ops, nil
	}
	if err := p.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// RemovePendingAppResources removes the pending application-level
// resources for a specific application, normally in the case that the
// application couldn't be deployed.
func (p *resourcePersistence) RemovePendingAppResources(applicationID string, pendingIDs map[string]string) error {
	buildTxn := func(int) ([]txn.Op, error) {
		return p.removePendingAppResourcesOps(applicationID, pendingIDs)
	}
	return errors.Trace(p.st.db().Run(buildTxn))
}

// resolveApplicationPendingResourcesOps generates mongo transaction operations
// to set the identified resources as active.
func (p *resourcePersistence) resolveApplicationPendingResourcesOps(applicationID string, pendingIDs map[string]string) ([]txn.Op, error) {
	rLogger.Tracef("resolve pending resource ops for %q", applicationID)
	if len(pendingIDs) == 0 {
		return nil, nil
	}

	// TODO(ericsnow) The resources need to be pulled in from the charm
	// store before we get to this point.

	var allOps []txn.Op
	for name, pendingID := range pendingIDs {
		resID := newAppResourceID(applicationID, name)
		ops, err := p.resolvePendingResourceOps(resID, pendingID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		allOps = append(allOps, ops...)
	}
	return allOps, nil
}

// resolvePendingResourceOps generates mongo transaction operations
// to set the identified resource as active.
func (p *resourcePersistence) resolvePendingResourceOps(resID, pendingID string) ([]txn.Op, error) {
	rLogger.Tracef("resolve pending resource ops %q, %q", resID, pendingID)
	if pendingID == "" {
		return nil, errors.New("missing pending ID")
	}

	oldDoc, err := p.getOnePending(resID, pendingID)
	if errors.IsNotFound(err) {
		return nil, errors.NotFoundf("pending resource %q (%s)", resID, pendingID)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	pending, err := doc2resource(oldDoc)
	if err != nil {
		return nil, errors.Trace(err)
	}

	exists := true
	if _, err := p.getOne(resID); errors.IsNotFound(err) {
		exists = false
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	csExists := true
	csResID := resID + resourcesCharmstoreIDSuffix
	if _, err := p.getOne(csResID); errors.IsNotFound(err) {
		csExists = false
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	ops := newResolvePendingResourceOps(pending, exists, csExists)
	return ops, nil
}

// removeUnitResourcesOps returns mgo transaction operations
// that remove resource information specific to the unit from state.
func (p *resourcePersistence) removeUnitResourcesOps(unitID string) ([]txn.Op, error) {
	docs, err := p.unitResources(unitID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ops := newRemoveResourcesOps(docs)
	// We do not remove the resource from the blob store here. That is
	// an application-level matter.
	return ops, nil
}

// removeResourcesOps returns mgo transaction operations that
// remove all the application's resources from state.
func (p *resourcePersistence) removeResourcesOps(applicationID string) ([]txn.Op, error) {
	docs, err := p.resources(applicationID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return removeResourcesAndStorageCleanupOps(docs), nil
}

// removePendingAppResourcesOps returns mgo transaction operations to
// clean up pending resources for the application from state. We pass
// in the pending IDs to avoid removing the wrong resources if there's
// a race to deploy the same application.
func (p *resourcePersistence) removePendingAppResourcesOps(applicationID string, pendingIDs map[string]string) ([]txn.Op, error) {
	docs, err := p.resources(applicationID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	pending := make([]resourceDoc, 0, len(docs))
	for _, doc := range docs {
		if doc.UnitID != "" || doc.PendingID == "" {
			continue
		}
		if pendingIDs[doc.Name] != doc.PendingID {
			// This is a pending resource for a different deployment
			// of an application with the same name.
			continue
		}
		pending = append(pending, doc)
	}
	return removeResourcesAndStorageCleanupOps(pending), nil
}

func applicationExistsOps(applicationID string) []txn.Op {
	return []txn.Op{{
		C:      applicationsC,
		Id:     applicationID,
		Assert: isAliveDoc,
	}}
}

func removeResourcesAndStorageCleanupOps(docs []resourceDoc) []txn.Op {
	ops := newRemoveResourcesOps(docs)
	seenPaths := set.NewStrings()
	for _, doc := range docs {
		// Don't schedule cleanups for placeholder resources, or multiple for a given path.
		if doc.StoragePath == "" || seenPaths.Contains(doc.StoragePath) {
			continue
		}
		ops = append(ops, newCleanupOp(cleanupResourceBlob, doc.StoragePath))
		seenPaths.Add(doc.StoragePath)
	}
	return ops
}

func newInsertStagedResourceOps(stored storedResource) []txn.Op {
	doc := newStagedResourceDoc(stored)

	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func newEnsureStagedResourceSameOps(stored storedResource) []txn.Op {
	doc := newStagedResourceDoc(stored)

	// Other than cause the txn to abort, we don't do anything here.
	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: doc, // TODO(ericsnow) Is this okay?
	}}
}

func newRemoveStagedResourceOps(id string) []txn.Op {
	fullID := stagedResourceID(id)

	// We don't assert that it exists. We want "missing" to be a noop.
	return []txn.Op{{
		C:      resourcesC,
		Id:     fullID,
		Remove: true,
	}}
}

func newInsertResourceOps(stored storedResource) []txn.Op {
	doc := newResourceDoc(stored)

	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func resourceDocToUpdateOp(doc *resourceDoc) bson.M {
	// Note (jam 2018-07-12): What are we actually allowed to update?
	// The old code was trying to delete the doc and replace it entirely, so for now we'll just set everything except
	// the doc's own id, which is clearly not allowed to change.
	return bson.M{"$set": bson.M{
		"resource-id":                doc.ID,
		"pending-id":                 doc.PendingID,
		"application-id":             doc.ApplicationID,
		"unit-id":                    doc.UnitID,
		"name":                       doc.Name,
		"type":                       doc.Type,
		"path":                       doc.Path,
		"description":                doc.Description,
		"origin":                     doc.Origin,
		"revision":                   doc.Revision,
		"fingerprint":                doc.Fingerprint,
		"size":                       doc.Size,
		"username":                   doc.Username,
		"timestamp-when-added":       doc.Timestamp,
		"storage-path":               doc.StoragePath,
		"download-progress":          doc.DownloadProgress,
		"timestamp-when-last-polled": doc.LastPolled.Round(time.Second).UTC(),
	}}
}

func newUpdateResourceOps(stored storedResource) []txn.Op {
	doc := newResourceDoc(stored)

	rLogger.Tracef("updating resource %s to %# v", stored.ID, pretty.Formatter(doc))
	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocExists,
		Update: resourceDocToUpdateOp(doc),
	}}
}

func newInsertCharmStoreResourceOps(res charmStoreResource) []txn.Op {
	doc := newCharmStoreResourceDoc(res)

	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func newUpdateCharmStoreResourceOps(res charmStoreResource) []txn.Op {
	doc := newCharmStoreResourceDoc(res)

	if rLogger.IsTraceEnabled() {
		rLogger.Tracef("updating charm store resource %s to %# v", res.id, pretty.Formatter(doc))
	}
	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocExists,
		Update: resourceDocToUpdateOp(doc),
	}}
}

func newInsertUnitResourceOps(unitID string, stored storedResource, progress *int64) []txn.Op {
	doc := newUnitResourceDoc(unitID, stored)
	doc.DownloadProgress = progress

	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
}

func newUpdateUnitResourceOps(unitID string, stored storedResource, progress *int64) []txn.Op {
	doc := newUnitResourceDoc(unitID, stored)
	doc.DownloadProgress = progress

	if rLogger.IsTraceEnabled() {
		rLogger.Tracef("updating unit resource %s to %# v", unitID, pretty.Formatter(doc))
	}
	return []txn.Op{{
		C:      resourcesC,
		Id:     doc.DocID,
		Assert: txn.DocExists, // feels like we need more
		Update: resourceDocToUpdateOp(doc),
	}}
}

func newRemoveResourcesOps(docs []resourceDoc) []txn.Op {
	// The likelihood of a race is small and the consequences are minor,
	// so we don't worry about the corner case of missing a doc here.
	var ops []txn.Op
	for _, doc := range docs {
		// We do not bother to assert txn.DocExists since it will be
		// gone either way, which is the result we're after.
		ops = append(ops, txn.Op{
			C:      resourcesC,
			Id:     doc.DocID,
			Remove: true,
		})
	}
	return ops
}

// newResolvePendingResourceOps generates transaction operations that
// will resolve a pending resource doc and make it active.
//
// We trust that the provided resource really is pending
// and that it matches the existing doc with the same ID.
func newResolvePendingResourceOps(pending storedResource, exists, csExists bool) []txn.Op {
	oldID := pendingResourceID(pending.ID, pending.PendingID)
	newRes := pending
	newRes.PendingID = ""
	// TODO(ericsnow) Update newRes.StoragePath? Doing so would require
	// moving the resource in the blobstore to the correct path, which
	// we cannot do in the transaction...
	ops := []txn.Op{{
		C:      resourcesC,
		Id:     oldID,
		Assert: txn.DocExists,
		Remove: true,
	}}

	// TODO(perrito666) 2016-05-02 lp:1558657
	csRes := charmStoreResource{
		Resource:      newRes.Resource.Resource,
		id:            newRes.ID,
		applicationID: newRes.ApplicationID,
		// Truncate the time to remove monotonic time for Go 1.9+
		// to make it easier for tests to compare the time.
		lastPolled: time.Now().Truncate(1).UTC(),
	}

	if exists {
		ops = append(ops, newUpdateResourceOps(newRes)...)

	} else {
		ops = append(ops, newInsertResourceOps(newRes)...)
	}
	if csExists {
		return append(ops, newUpdateCharmStoreResourceOps(csRes)...)
	} else {
		return append(ops, newInsertCharmStoreResourceOps(csRes)...)
	}
}
