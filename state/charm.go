// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"net/url"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/storage"
)

// charmDoc represents the internal state of a charm in MongoDB.
type charmDoc struct {
	DocID   string     `bson:"_id"`
	URL     *charm.URL `bson:"url"` // DANGEROUS see below
	EnvUUID string     `bson:"env-uuid"`

	// TODO(fwereade) 2015-06-18 lp:1467964
	// DANGEROUS: our schema can change any time the charm package changes,
	// and we have no automated way to detect when that happens. We *must*
	// not depend upon serializations we cannot control from inside this
	// package. What's in a *charm.Meta? What will be tomorrow? What logic
	// will we be writing on the assumption that all stored Metas have set
	// some field? What fields might lose precision when they go into the
	// database?
	Meta    *charm.Meta    `bson:"meta"`
	Config  *charm.Config  `bson:"config"`
	Actions *charm.Actions `bson:"actions"`
	Metrics *charm.Metrics `bson:"metrics"`

	// DEPRECATED: BundleURL is deprecated, and exists here
	// only for migration purposes. We should remove this
	// when migrations are no longer necessary.
	BundleURL *url.URL `bson:"bundleurl,omitempty"`

	BundleSha256  string `bson:"bundlesha256"`
	StoragePath   string `bson:"storagepath"`
	PendingUpload bool   `bson:"pendingupload"`
	Placeholder   bool   `bson:"placeholder"`
}

// insertCharmOps returns the txn operations necessary to insert the supplied
// charm data.
func insertCharmOps(
	st *State, ch charm.Charm, curl *charm.URL, storagePath, bundleSha256 string,
) ([]txn.Op, error) {
	return insertAnyCharmOps(&charmDoc{
		DocID:        curl.String(),
		URL:          curl,
		EnvUUID:      st.EnvironTag().Id(),
		Meta:         ch.Meta(),
		Config:       safeConfig(ch),
		Metrics:      ch.Metrics(),
		Actions:      ch.Actions(),
		BundleSha256: bundleSha256,
		StoragePath:  storagePath,
	})
}

// insertPlaceholderCharmOps returns the txn operations necessary to insert a
// charm document referencing a store charm that is not yet directly accessible
// within the environment.
func insertPlaceholderCharmOps(st *State, curl *charm.URL) ([]txn.Op, error) {
	return insertAnyCharmOps(&charmDoc{
		DocID:       curl.String(),
		URL:         curl,
		EnvUUID:     st.EnvironTag().Id(),
		Placeholder: true,
	})
}

// insertPendingCharmOps returns the txn operations necessary to insert a charm
// document referencing a charm that has yet to be uploaded to the environment.
func insertPendingCharmOps(st *State, curl *charm.URL) ([]txn.Op, error) {
	return insertAnyCharmOps(&charmDoc{
		DocID:         curl.String(),
		URL:           curl,
		EnvUUID:       st.EnvironTag().Id(),
		PendingUpload: true,
	})
}

// insertAnyCharmOps returns the txn operations necessary to insert the supplied
// charm document.
func insertAnyCharmOps(cdoc *charmDoc) ([]txn.Op, error) {
	return []txn.Op{{
		C:      charmsC,
		Id:     cdoc.DocID,
		Assert: txn.DocMissing,
		Insert: cdoc,
	}}, nil
}

// updateCharmOps returns the txn operations necessary to update the charm
// document with the supplied data, so long as the supplied assert still holds
// true.
func updateCharmOps(
	st *State, ch charm.Charm, curl *charm.URL, storagePath, bundleSha256 string, assert bson.D,
) ([]txn.Op, error) {

	updateFields := bson.D{{"$set", bson.D{
		{"meta", ch.Meta()},
		{"config", safeConfig(ch)},
		{"actions", ch.Actions()},
		{"metrics", ch.Metrics()},
		{"storagepath", storagePath},
		{"bundlesha256", bundleSha256},
		{"pendingupload", false},
		{"placeholder", false},
	}}}
	return []txn.Op{{
		C:      charmsC,
		Id:     curl.String(),
		Assert: assert,
		Update: updateFields,
	}}, nil
}

// convertPlaceholderCharmOps returns the txn operations necessary to convert
// the charm with the supplied docId from a placeholder to one marked for
// pending upload.
func convertPlaceholderCharmOps(docID string) ([]txn.Op, error) {
	return []txn.Op{{
		C:  charmsC,
		Id: docID,
		Assert: bson.D{
			{"bundlesha256", ""},
			{"pendingupload", false},
			{"placeholder", true},
		},
		Update: bson.D{{"$set", bson.D{
			{"pendingupload", true},
			{"placeholder", false},
		}}},
	}}, nil

}

// deleteOldPlaceholderCharmsOps returns the txn ops required to delete all placeholder charm
// records older than the specified charm URL.
func deleteOldPlaceholderCharmsOps(st *State, charms mongo.Collection, curl *charm.URL) ([]txn.Op, error) {
	// Get a regex with the charm URL and no revision.
	noRevURL := curl.WithRevision(-1)
	curlRegex := "^" + regexp.QuoteMeta(st.docID(noRevURL.String()))

	var docs []charmDoc
	query := bson.D{{"_id", bson.D{{"$regex", curlRegex}}}, {"placeholder", true}}
	err := charms.Find(query).Select(bson.D{{"_id", 1}, {"url", 1}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var ops []txn.Op
	for _, doc := range docs {
		if doc.URL.Revision >= curl.Revision {
			continue
		}
		ops = append(ops, txn.Op{
			C:      charmsC,
			Id:     doc.DocID,
			Assert: stillPlaceholder,
			Remove: true,
		})
	}
	return ops, nil
}

// safeConfig is a travesty which attempts to work around our continued failure
// to properly insulate our database from code changes; it escapes mongo-
// significant characters in config options. See lp:1467964.
func safeConfig(ch charm.Charm) *charm.Config {
	// Make sure we escape any "$" and "." in config option names
	// first. See http://pad.lv/1308146.
	cfg := ch.Config()
	escapedConfig := charm.NewConfig()
	for optionName, option := range cfg.Options {
		escapedName := escapeReplacer.Replace(optionName)
		escapedConfig.Options[escapedName] = option
	}
	return escapedConfig
}

// Charm represents the state of a charm in the environment.
type Charm struct {
	st  *State
	doc charmDoc
}

func newCharm(st *State, cdoc *charmDoc) *Charm {
	// Because we probably just read the doc from state, make sure we
	// unescape any config option names for "$" and ".". See
	// http://pad.lv/1308146
	if cdoc != nil && cdoc.Config != nil {
		unescapedConfig := charm.NewConfig()
		for optionName, option := range cdoc.Config.Options {
			unescapedName := unescapeReplacer.Replace(optionName)
			unescapedConfig.Options[unescapedName] = option
		}
		cdoc.Config = unescapedConfig
	}
	return &Charm{st: st, doc: *cdoc}
}

// Tag returns a tag identifying the charm.
// Implementing state.GlobalEntity interface.
func (c *Charm) Tag() names.Tag {
	return names.NewCharmTag(c.URL().String())
}

// charmGlobalKey returns the global database key for the charm
// with the given url.
func charmGlobalKey(charmURL *charm.URL) string {
	return "c#" + charmURL.String()
}

// GlobalKey returns the global database key for the charm.
// Implementing state.GlobalEntity interface.
func (c *Charm) globalKey() string {
	return charmGlobalKey(c.doc.URL)
}

func (c *Charm) String() string {
	return c.doc.URL.String()
}

// URL returns the URL that identifies the charm.
func (c *Charm) URL() *charm.URL {
	clone := *c.doc.URL
	return &clone
}

// Revision returns the monotonically increasing charm
// revision number.
func (c *Charm) Revision() int {
	return c.doc.URL.Revision
}

// Meta returns the metadata of the charm.
func (c *Charm) Meta() *charm.Meta {
	return c.doc.Meta
}

// Config returns the configuration of the charm.
func (c *Charm) Config() *charm.Config {
	return c.doc.Config
}

// Metrics returns the metrics declared for the charm.
func (c *Charm) Metrics() *charm.Metrics {
	return c.doc.Metrics
}

// Actions returns the actions definition of the charm.
func (c *Charm) Actions() *charm.Actions {
	return c.doc.Actions
}

// StoragePath returns the storage path of the charm bundle.
func (c *Charm) StoragePath() string {
	return c.doc.StoragePath
}

// BundleURL returns the url to the charm bundle in
// the provider storage.
//
// DEPRECATED: this is only to be used for migrating
// charm archives to environment storage.
func (c *Charm) BundleURL() *url.URL {
	return c.doc.BundleURL
}

// BundleSha256 returns the SHA256 digest of the charm bundle bytes.
func (c *Charm) BundleSha256() string {
	return c.doc.BundleSha256
}

// IsUploaded returns whether the charm has been uploaded to the
// environment storage.
func (c *Charm) IsUploaded() bool {
	return !c.doc.PendingUpload
}

// IsPlaceholder returns whether the charm record is just a placeholder
// rather than representing a deployed charm.
func (c *Charm) IsPlaceholder() bool {
	return c.doc.Placeholder
}

// deleteCharmArchive deletes a charm archive from blob storage
// and removes the corresponding charm record from state.
func (st *State) deleteCharmArchive(curl *charm.URL, storagePath string) error {
	if err := st.deleteCharm(curl); err != nil {
		return errors.Annotate(err, "cannot delete charm record from state")
	}
	stor := storage.NewStorage(st.EnvironUUID(), st.MongoSession())
	if err := stor.Remove(storagePath); err != nil {
		return errors.Annotate(err, "cannot delete charm from storage")
	}
	return nil
}

// AddCharm adds the ch charm with curl to the state.
// On success the newly added charm state is returned.
func (st *State) AddCharm(ch charm.Charm, curl *charm.URL, storagePath, bundleSha256 string) (stch *Charm, err error) {
	charms, closer := st.getCollection(charmsC)
	defer closer()

	query := charms.FindId(curl.String()).Select(bson.D{{"placeholder", 1}})

	buildTxn := func(attempt int) ([]txn.Op, error) {
		var placeholderDoc struct {
			Placeholder bool `bson:"placeholder"`
		}
		if err := query.One(&placeholderDoc); err == mgo.ErrNotFound {
			return insertCharmOps(st, ch, curl, storagePath, bundleSha256)
		} else if err != nil {
			return nil, errors.Trace(err)
		} else if placeholderDoc.Placeholder {
			return updateCharmOps(st, ch, curl, storagePath, bundleSha256, stillPlaceholder)
		}
		return nil, errors.AlreadyExistsf("charm %q", curl)
	}
	if err = st.run(buildTxn); err == nil {
		return st.Charm(curl)
	}
	return nil, errors.Trace(err)
}

// deleteCharm removes the charm record with curl from state.
func (st *State) deleteCharm(curl *charm.URL) error {
	op := []txn.Op{{
		C:      charmsC,
		Id:     curl.String(),
		Remove: true,
	}}
	err := st.runTransaction(op)
	if err == mgo.ErrNotFound {
		return nil
	}
	return errors.Trace(err)
}

type hasMeta interface {
	Meta() *charm.Meta
}

// AllCharms returns all charms in state.
func (st *State) AllCharms() ([]*Charm, error) {
	charmsCollection, closer := st.getCollection(charmsC)
	defer closer()
	var cdoc charmDoc
	var charms []*Charm
	iter := charmsCollection.Find(nil).Iter()
	for iter.Next(&cdoc) {
		charms = append(charms, newCharm(st, &cdoc))
	}
	return charms, errors.Trace(iter.Close())
}

// Charm returns the charm with the given URL. Charms pending upload
// to storage and placeholders are never returned.
func (st *State) Charm(curl *charm.URL) (*Charm, error) {
	charms, closer := st.getCollection(charmsC)
	defer closer()

	cdoc := &charmDoc{}
	what := bson.D{
		{"_id", curl.String()},
		{"placeholder", bson.D{{"$ne", true}}},
		{"pendingupload", bson.D{{"$ne", true}}},
	}
	err := charms.Find(what).One(&cdoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("charm %q", curl)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get charm %q", curl)
	}
	if err := cdoc.Meta.Check(); err != nil {
		return nil, errors.Annotatef(err, "malformed charm metadata found in state")
	}
	return newCharm(st, cdoc), nil
}

// LatestPlaceholderCharm returns the latest charm described by the
// given URL but which is not yet deployed.
func (st *State) LatestPlaceholderCharm(curl *charm.URL) (*Charm, error) {
	charms, closer := st.getCollection(charmsC)
	defer closer()

	noRevURL := curl.WithRevision(-1)
	curlRegex := "^" + regexp.QuoteMeta(st.docID(noRevURL.String()))
	var docs []charmDoc
	err := charms.Find(bson.D{{"_id", bson.D{{"$regex", curlRegex}}}, {"placeholder", true}}).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get charm %q", curl)
	}
	// Find the highest revision.
	var latest charmDoc
	for _, doc := range docs {
		if latest.URL == nil || doc.URL.Revision > latest.URL.Revision {
			latest = doc
		}
	}
	if latest.URL == nil {
		return nil, errors.NotFoundf("placeholder charm %q", noRevURL)
	}
	return newCharm(st, &latest), nil
}

// PrepareLocalCharmUpload must be called before a local charm is
// uploaded to the provider storage in order to create a charm
// document in state. It returns the chosen unique charm URL reserved
// in state for the charm.
//
// The url's schema must be "local" and it must include a revision.
func (st *State) PrepareLocalCharmUpload(curl *charm.URL) (chosenUrl *charm.URL, err error) {
	// Perform a few sanity checks first.
	if curl.Schema != "local" {
		return nil, errors.Errorf("expected charm URL with local schema, got %q", curl)
	}
	if curl.Revision < 0 {
		return nil, errors.Errorf("expected charm URL with revision, got %q", curl)
	}
	// Get a regex with the charm URL and no revision.
	noRevURL := curl.WithRevision(-1)
	curlRegex := "^" + regexp.QuoteMeta(st.docID(noRevURL.String()))

	charms, closer := st.getCollection(charmsC)
	defer closer()

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// Find the highest revision of that charm in state.
		var docs []charmDoc
		query := bson.D{{"_id", bson.D{{"$regex", curlRegex}}}}
		err = charms.Find(query).Select(bson.D{{"_id", 1}, {"url", 1}}).All(&docs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// Find the highest revision.
		maxRevision := -1
		for _, doc := range docs {
			if doc.URL.Revision > maxRevision {
				maxRevision = doc.URL.Revision
			}
		}

		// Respect the local charm's revision first.
		chosenRevision := curl.Revision
		if maxRevision >= chosenRevision {
			// More recent revision exists in state, pick the next.
			chosenRevision = maxRevision + 1
		}
		chosenUrl = curl.WithRevision(chosenRevision)
		return insertPendingCharmOps(st, chosenUrl)
	}
	if err = st.run(buildTxn); err == nil {
		return chosenUrl, nil
	}
	return nil, errors.Trace(err)
}

// PrepareStoreCharmUpload must be called before a charm store charm
// is uploaded to the provider storage in order to create a charm
// document in state. If a charm with the same URL is already in
// state, it will be returned as a *state.Charm (it can be still
// pending or already uploaded). Otherwise, a new charm document is
// added in state with just the given charm URL and
// PendingUpload=true, which is then returned as a *state.Charm.
//
// The url's schema must be "cs" and it must include a revision.
func (st *State) PrepareStoreCharmUpload(curl *charm.URL) (*Charm, error) {
	// Perform a few sanity checks first.
	if curl.Schema != "cs" {
		return nil, errors.Errorf("expected charm URL with cs schema, got %q", curl)
	}
	if curl.Revision < 0 {
		return nil, errors.Errorf("expected charm URL with revision, got %q", curl)
	}

	charms, closer := st.getCollection(charmsC)
	defer closer()

	var (
		uploadedCharm charmDoc
		err           error
	)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// Find an uploaded or pending charm with the given exact curl.
		err := charms.FindId(curl.String()).One(&uploadedCharm)
		switch {
		case err == mgo.ErrNotFound:
			uploadedCharm = charmDoc{
				DocID:         st.docID(curl.String()),
				EnvUUID:       st.EnvironTag().Id(),
				URL:           curl,
				PendingUpload: true,
			}
			return insertAnyCharmOps(&uploadedCharm)
		case err != nil:
			return nil, errors.Trace(err)
		case uploadedCharm.Placeholder:
			// Update the fields of the document we're returning.
			uploadedCharm.PendingUpload = true
			uploadedCharm.Placeholder = false
			return convertPlaceholderCharmOps(uploadedCharm.DocID)
		default:
			// The charm exists and it's either uploaded or still
			// pending, but it's not a placeholder. In any case,
			// there's nothing to do.
			return nil, jujutxn.ErrNoOperations
		}
	}
	if err = st.run(buildTxn); err == nil {
		return newCharm(st, &uploadedCharm), nil
	}
	return nil, errors.Trace(err)
}

var (
	stillPending     = bson.D{{"pendingupload", true}}
	stillPlaceholder = bson.D{{"placeholder", true}}
)

// AddStoreCharmPlaceholder creates a charm document in state for the given charm URL which
// must reference a charm from the store. The charm document is marked as a placeholder which
// means that if the charm is to be deployed, it will need to first be uploaded to env storage.
func (st *State) AddStoreCharmPlaceholder(curl *charm.URL) (err error) {
	// Perform sanity checks first.
	if curl.Schema != "cs" {
		return errors.Errorf("expected charm URL with cs schema, got %q", curl)
	}
	if curl.Revision < 0 {
		return errors.Errorf("expected charm URL with revision, got %q", curl)
	}
	charms, closer := st.getCollection(charmsC)
	defer closer()

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// See if the charm already exists in state and exit early if that's the case.
		var doc charmDoc
		err := charms.Find(bson.D{{"_id", curl.String()}}).Select(bson.D{{"_id", 1}}).One(&doc)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		if err == nil {
			return nil, jujutxn.ErrNoOperations
		}

		// Delete all previous placeholders so we don't fill up the database with unused data.
		deleteOps, err := deleteOldPlaceholderCharmsOps(st, charms, curl)
		if err != nil {
			return nil, errors.Trace(err)
		}
		insertOps, err := insertPlaceholderCharmOps(st, curl)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops := append(deleteOps, insertOps...)
		return ops, nil
	}
	return errors.Trace(st.run(buildTxn))
}

// UpdateUploadedCharm marks the given charm URL as uploaded and
// updates the rest of its data, returning it as *state.Charm.
func (st *State) UpdateUploadedCharm(ch charm.Charm, curl *charm.URL, storagePath, bundleSha256 string) (*Charm, error) {
	charms, closer := st.getCollection(charmsC)
	defer closer()

	doc := &charmDoc{}
	err := charms.FindId(curl.String()).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("charm %q", curl)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !doc.PendingUpload {
		return nil, errors.Trace(&ErrCharmAlreadyUploaded{curl})
	}

	ops, err := updateCharmOps(st, ch, curl, storagePath, bundleSha256, stillPending)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := st.runTransaction(ops); err != nil {
		return nil, onAbort(err, ErrCharmRevisionAlreadyModified)
	}
	return st.Charm(curl)
}
