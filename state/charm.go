// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"regexp"
	"strings"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jujutxn "github.com/juju/txn"
	"gopkg.in/macaroon.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/mongo"
	mongoutils "github.com/juju/juju/mongo/utils"
	"github.com/juju/juju/state/storage"
	jujuversion "github.com/juju/juju/version"
)

// MacaroonCache is a type that wraps State and implements charmstore.MacaroonCache.
type MacaroonCache struct {
	*State
}

// Set stores the macaroon on the charm.
func (m MacaroonCache) Set(u *charm.URL, ms macaroon.Slice) error {
	c, err := m.Charm(u)
	if err != nil {
		return errors.Trace(err)
	}
	return c.UpdateMacaroon(ms)
}

// Get retrieves the macaroon for the charm (if any).
func (m MacaroonCache) Get(u *charm.URL) (macaroon.Slice, error) {
	c, err := m.Charm(u)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.Macaroon()
}

// charmDoc represents the internal state of a charm in MongoDB.
type charmDoc struct {
	ModelUUID string     `bson:"model-uuid"`
	DocID     string     `bson:"_id"`
	URL       *charm.URL `bson:"url"` // DANGEROUS see charm.* fields below
	// CharmVersion
	CharmVersion string `bson:"charm-version"`

	// Life manages charm lifetime in the usual way, but only local
	// charms can actually be "destroyed"; store charms are
	// immortal.
	Life Life `bson:"life"`

	// These fields are flags; if any of them is set, the charm
	// cannot actually be safely used for anything.
	PendingUpload bool `bson:"pendingupload"`
	Placeholder   bool `bson:"placeholder"`

	// These fields control access to the charm archive.
	BundleSha256 string `bson:"bundlesha256"`
	StoragePath  string `bson:"storagepath"`
	Macaroon     []byte `bson:"macaroon"`

	// The remaining fields hold data sufficient to define a
	// charm.Charm.

	// TODO(fwereade) 2015-06-18 lp:1467964
	// DANGEROUS: our schema can change any time the charm package changes,
	// and we have no automated way to detect when that happens. We *must*
	// not depend upon serializations we cannot control from inside this
	// package. What's in a *charm.Meta? What will be tomorrow? What logic
	// will we be writing on the assumption that all stored Metas have set
	// some field? What fields might lose precision when they go into the
	// database?
	Meta       *charm.Meta       `bson:"meta"`
	Config     *charm.Config     `bson:"config"`
	Actions    *charm.Actions    `bson:"actions"`
	Metrics    *charm.Metrics    `bson:"metrics"`
	LXDProfile *charm.LXDProfile `bson:"lxd-profile"`
}

// CharmInfo contains all the data necessary to store a charm's metadata.
type CharmInfo struct {
	Charm       charm.Charm
	ID          *charm.URL
	StoragePath string
	SHA256      string
	Macaroon    macaroon.Slice
	Version     string
}

// insertCharmOps returns the txn operations necessary to insert the supplied
// charm data. If curl is nil, an error will be returned.
func insertCharmOps(mb modelBackend, info CharmInfo) ([]txn.Op, error) {
	if info.ID == nil {
		return nil, errors.New("*charm.URL was nil")
	}

	doc := charmDoc{
		DocID:        info.ID.String(),
		URL:          info.ID,
		CharmVersion: info.Version,
		Meta:         info.Charm.Meta(),
		Config:       safeConfig(info.Charm),
		Metrics:      info.Charm.Metrics(),
		Actions:      info.Charm.Actions(),
		BundleSha256: info.SHA256,
		StoragePath:  info.StoragePath,
	}
	lpc, ok := info.Charm.(charm.LXDProfiler)
	if !ok {
		return nil, errors.New("charm does no implement LXDProfiler")
	}
	doc.LXDProfile = safeLXDProfile(lpc.LXDProfile())

	if err := checkCharmDataIsStorable(doc); err != nil {
		return nil, errors.Trace(err)
	}

	if info.Macaroon != nil {
		mac, err := info.Macaroon.MarshalBinary()
		if err != nil {
			return nil, errors.Annotate(err, "can't convert macaroon to binary for storage")
		}
		doc.Macaroon = mac
	}
	return insertAnyCharmOps(mb, &doc)
}

// insertPlaceholderCharmOps returns the txn operations necessary to insert a
// charm document referencing a store charm that is not yet directly accessible
// within the model. If curl is nil, an error will be returned.
func insertPlaceholderCharmOps(mb modelBackend, curl *charm.URL) ([]txn.Op, error) {
	if curl == nil {
		return nil, errors.New("*charm.URL was nil")
	}
	return insertAnyCharmOps(mb, &charmDoc{
		DocID:       curl.String(),
		URL:         curl,
		Placeholder: true,
	})
}

// insertPendingCharmOps returns the txn operations necessary to insert a charm
// document referencing a charm that has yet to be uploaded to the model.
// If curl is nil, an error will be returned.
func insertPendingCharmOps(mb modelBackend, curl *charm.URL) ([]txn.Op, error) {
	if curl == nil {
		return nil, errors.New("*charm.URL was nil")
	}
	return insertAnyCharmOps(mb, &charmDoc{
		DocID:         curl.String(),
		URL:           curl,
		PendingUpload: true,
	})
}

// insertAnyCharmOps returns the txn operations necessary to insert the supplied
// charm document.
func insertAnyCharmOps(mb modelBackend, cdoc *charmDoc) ([]txn.Op, error) {
	charms, closer := mb.db().GetCollection(charmsC)
	defer closer()

	life, err := nsLife.read(charms, cdoc.DocID)
	if errors.IsNotFound(err) {
		// everything is as it should be
	} else if err != nil {
		return nil, errors.Trace(err)
	} else if life == Dead {
		return nil, errors.New("url already consumed")
	} else {
		return nil, errors.New("already exists")
	}
	charmOp := txn.Op{
		C:      charmsC,
		Id:     cdoc.DocID,
		Assert: txn.DocMissing,
		Insert: cdoc,
	}

	refcounts, closer := mb.db().GetCollection(refcountsC)
	defer closer()

	charmKey := charmGlobalKey(cdoc.URL)
	refOp, required, err := nsRefcounts.LazyCreateOp(refcounts, charmKey)
	if err != nil {
		return nil, errors.Trace(err)
	} else if required {
		return []txn.Op{refOp, charmOp}, nil
	}
	return []txn.Op{charmOp}, nil
}

// updateCharmOps returns the txn operations necessary to update the charm
// document with the supplied data, so long as the supplied assert still holds
// true.
func updateCharmOps(mb modelBackend, info CharmInfo, assert bson.D) ([]txn.Op, error) {
	charms, closer := mb.db().GetCollection(charmsC)
	defer closer()

	charmKey := info.ID.String()
	op, err := nsLife.aliveOp(charms, charmKey)
	if err != nil {
		return nil, errors.Annotate(err, "charm")
	}
	lifeAssert, ok := op.Assert.(bson.D)
	if !ok {
		return nil, errors.Errorf("expected bson.D, got %#v", op.Assert)
	}
	op.Assert = append(lifeAssert, assert...)

	data := bson.D{
		{"charm-version", info.Version},
		{"meta", info.Charm.Meta()},
		{"config", safeConfig(info.Charm)},
		{"actions", info.Charm.Actions()},
		{"metrics", info.Charm.Metrics()},
		{"storagepath", info.StoragePath},
		{"bundlesha256", info.SHA256},
		{"pendingupload", false},
		{"placeholder", false},
	}

	lpc, ok := info.Charm.(charm.LXDProfiler)
	if !ok {
		return nil, errors.New("charm doesn't have LXDCharmProfile()")
	}
	data = append(data, bson.DocElem{"lxd-profile", safeLXDProfile(lpc.LXDProfile())})

	if err := checkCharmDataIsStorable(data); err != nil {
		return nil, errors.Trace(err)
	}

	if len(info.Macaroon) > 0 {
		mac, err := info.Macaroon.MarshalBinary()
		if err != nil {
			return nil, errors.Annotate(err, "can't convert macaroon to binary for storage")
		}
		data = append(data, bson.DocElem{"macaroon", mac})
	}

	op.Update = bson.D{{"$set", data}}
	return []txn.Op{op}, nil
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
func deleteOldPlaceholderCharmsOps(mb modelBackend, charms mongo.Collection, curl *charm.URL) ([]txn.Op, error) {
	// Get a regex with the charm URL and no revision.
	noRevURL := curl.WithRevision(-1)
	curlRegex := "^" + regexp.QuoteMeta(mb.docID(noRevURL.String()))

	var docs []charmDoc
	query := bson.D{{"_id", bson.D{{"$regex", curlRegex}}}, {"placeholder", true}}
	err := charms.Find(query).Select(bson.D{{"_id", 1}, {"url", 1}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	refcounts, closer := mb.db().GetCollection(refcountsC)
	defer closer()

	var ops []txn.Op
	for _, doc := range docs {
		if doc.URL.Revision >= curl.Revision {
			continue
		}
		key := charmGlobalKey(doc.URL)
		refOp, err := nsRefcounts.RemoveOp(refcounts, key, 0)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, refOp, txn.Op{
			C:      charms.Name(),
			Id:     doc.DocID,
			Assert: stillPlaceholder,
			Remove: true,
		})
	}
	return ops, nil
}

func checkCharmDataIsStorable(charmData interface{}) error {
	err := mongoutils.CheckStorable(charmData)
	return errors.Annotate(err, "invalid charm data")
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
		escapedName := mongoutils.EscapeKey(optionName)
		escapedConfig.Options[escapedName] = option
	}
	return escapedConfig
}

// safeLXDProfile ensures that the LXDProfile that we put into the mongo data
// store, can in fact store the profile safely by escaping mongo-
// significant characters in config options.
func safeLXDProfile(profile *charm.LXDProfile) *charm.LXDProfile {
	if profile == nil {
		return nil
	}
	escapedProfile := charm.NewLXDProfile()
	escapedProfile.Description = profile.Description
	// we know the size and shape of the type, so let's use EscapeKey
	escapedConfig := make(map[string]string, len(profile.Config))
	for k, v := range profile.Config {
		escapedConfig[mongoutils.EscapeKey(k)] = v
	}
	escapedProfile.Config = escapedConfig
	// this is more easy to reason about than using mongoutils.EscapeKeys, which
	// requires looping from map[string]interface{} -> map[string]map[string]string
	escapedDevices := make(map[string]map[string]string, len(profile.Devices))
	for k, v := range profile.Devices {
		nested := make(map[string]string, len(v))
		for vk, vv := range v {
			nested[mongoutils.EscapeKey(vk)] = vv
		}
		escapedDevices[mongoutils.EscapeKey(k)] = nested
	}
	escapedProfile.Devices = escapedDevices
	return escapedProfile
}

// Charm represents the state of a charm in the model.
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
			unescapedName := mongoutils.UnescapeKey(optionName)
			unescapedConfig.Options[unescapedName] = option
		}
		cdoc.Config = unescapedConfig
	}

	if cdoc != nil {
		cdoc.LXDProfile = unescapeLXDProfile(cdoc.LXDProfile)
	}

	cdoc.ModelUUID = st.ModelUUID()

	ch := Charm{st: st, doc: *cdoc}
	return &ch
}

// unescapeLXDProfile returns the LXDProfile back to normal after
// reading from state.
func unescapeLXDProfile(profile *charm.LXDProfile) *charm.LXDProfile {
	if profile == nil {
		return nil
	}
	unescapedProfile := charm.NewLXDProfile()
	unescapedProfile.Description = profile.Description
	// we know the size and shape of the type, so let's use UnescapeKey
	unescapedConfig := make(map[string]string, len(profile.Config))
	for k, v := range profile.Config {
		unescapedConfig[mongoutils.UnescapeKey(k)] = v
	}
	unescapedProfile.Config = unescapedConfig
	// this is more easy to reason about than using mongoutils.UnescapeKeys, which
	// requires looping from map[string]interface{} -> map[string]map[string]string
	unescapedDevices := make(map[string]map[string]string, len(profile.Devices))
	for k, v := range profile.Devices {
		nested := make(map[string]string, len(v))
		for vk, vv := range v {
			nested[mongoutils.UnescapeKey(vk)] = vv
		}
		unescapedDevices[mongoutils.UnescapeKey(k)] = nested
	}
	unescapedProfile.Devices = unescapedDevices
	return unescapedProfile
}

// Tag returns a tag identifying the charm.
// Implementing state.GlobalEntity interface.
func (c *Charm) Tag() names.Tag {
	return names.NewCharmTag(c.URL().String())
}

// Life returns the charm's life state.
func (c *Charm) Life() Life {
	return c.doc.Life
}

// Refresh loads fresh charm data from the database. In practice, the
// only observable change should be to its Life value.
func (c *Charm) Refresh() error {
	ch, err := c.st.Charm(c.doc.URL)
	if err != nil {
		return errors.Trace(err)
	}
	c.doc = ch.doc
	return nil
}

// Destroy sets the charm to Dying and prevents it from being used by
// applications or units. It only works on local charms, and only when
// the charm is not referenced by any application.
func (c *Charm) Destroy() error {
	buildTxn := func(_ int) ([]txn.Op, error) {
		ops, err := charmDestroyOps(c.st, c.doc.URL)
		if IsNotAlive(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	}
	if err := c.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	c.doc.Life = Dying
	return nil
}

// Remove will delete the charm's stored archive and render the charm
// inaccessible to future clients. It will fail unless the charm is
// already Dying (indicating that someone has called Destroy).
func (c *Charm) Remove() error {
	if c.doc.Life == Alive {
		return errors.New("still alive")
	}

	stor := storage.NewStorage(c.st.ModelUUID(), c.st.MongoSession())
	err := stor.Remove(c.doc.StoragePath)
	if errors.IsNotFound(err) {
		// Not a problem, but we might still need to run the
		// transaction further down to complete the process.
	} else if err != nil {
		return errors.Annotate(err, "deleting archive")
	}

	// We know the charm is already dying, dead or removed at this
	// point (life can *never* go backwards) so an unasserted remove
	// is safe.
	removeOps := []txn.Op{{
		C:      charmsC,
		Id:     c.doc.URL.String(),
		Remove: true,
	}}
	if err := c.st.db().RunTransaction(removeOps); err != nil {
		return errors.Trace(err)
	}
	c.doc.Life = Dead
	return nil
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

// Version returns the charm version.
func (c *Charm) Version() string {
	return c.doc.CharmVersion
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

// LXDProfile returns the lxd profile definition of the charm.
func (c *Charm) LXDProfile() *charm.LXDProfile {
	return c.doc.LXDProfile
}

// StoragePath returns the storage path of the charm bundle.
func (c *Charm) StoragePath() string {
	return c.doc.StoragePath
}

// BundleSha256 returns the SHA256 digest of the charm bundle bytes.
func (c *Charm) BundleSha256() string {
	return c.doc.BundleSha256
}

// IsUploaded returns whether the charm has been uploaded to the
// model storage.
func (c *Charm) IsUploaded() bool {
	return !c.doc.PendingUpload
}

// IsPlaceholder returns whether the charm record is just a placeholder
// rather than representing a deployed charm.
func (c *Charm) IsPlaceholder() bool {
	return c.doc.Placeholder
}

// Macaroon return the macaroon that can be used to request data about the charm
// from the charmstore, or nil if the charm is not private.
func (c *Charm) Macaroon() (macaroon.Slice, error) {
	if len(c.doc.Macaroon) == 0 {
		return nil, nil
	}
	var m macaroon.Slice
	if err := m.UnmarshalBinary(c.doc.Macaroon); err != nil {
		return nil, errors.Trace(err)
	}

	return m, nil
}

// UpdateMacaroon updates the stored macaroon for this charm.
func (c *Charm) UpdateMacaroon(m macaroon.Slice) error {
	info := CharmInfo{
		Charm:       c,
		ID:          c.URL(),
		StoragePath: c.StoragePath(),
		SHA256:      c.BundleSha256(),
		Macaroon:    m,
	}
	ops, err := updateCharmOps(c.st, info, nil)
	if err != nil {
		return errors.Trace(err)
	}
	if err := c.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// AddCharm adds the ch charm with curl to the state.
// On success the newly added charm state is returned.
func (st *State) AddCharm(info CharmInfo) (stch *Charm, err error) {
	charms, closer := st.db().GetCollection(charmsC)
	defer closer()

	if err := jujuversion.CheckJujuMinVersion(info.Charm.Meta().MinJujuVersion, jujuversion.Current); err != nil {
		return nil, errors.Trace(err)
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := validateCharmSeries(model.Type(), info.ID.Series, info.Charm); err != nil {
		return nil, errors.Trace(err)
	}

	query := charms.FindId(info.ID.String()).Select(bson.M{
		"placeholder":   1,
		"pendingupload": 1,
	})
	buildTxn := func(attempt int) ([]txn.Op, error) {
		var doc charmDoc
		if err := query.One(&doc); err == mgo.ErrNotFound {
			if info.ID.Schema == "local" {
				curl, err := st.PrepareLocalCharmUpload(info.ID)
				if err != nil {
					return nil, errors.Trace(err)
				}
				info.ID = curl
				return updateCharmOps(st, info, stillPending)
			}
			return insertCharmOps(st, info)
		} else if err != nil {
			return nil, errors.Trace(err)
		} else if doc.PendingUpload {
			return updateCharmOps(st, info, stillPending)
		} else if doc.Placeholder {
			return updateCharmOps(st, info, stillPlaceholder)
		}
		return nil, errors.AlreadyExistsf("charm %q", info.ID)
	}
	if err = st.db().Run(buildTxn); err == nil {
		return st.Charm(info.ID)
	}
	return nil, errors.Trace(err)
}

type hasMeta interface {
	Meta() *charm.Meta
}

func validateCharmSeries(modelType ModelType, series string, ch hasMeta) error {
	if series == "" {
		allSeries := ch.Meta().Series
		if len(allSeries) > 0 {
			series = allSeries[0]
		}
	}
	// TODO(wallyworld) - update lots-o-tests
	// Some tests don't set a series.
	if series == "" {
		return nil
	}
	return model.ValidateSeries(model.ModelType(modelType), series)
}

// AllCharms returns all charms in state.
func (st *State) AllCharms() ([]*Charm, error) {
	charmsCollection, closer := st.db().GetCollection(charmsC)
	defer closer()
	var cdoc charmDoc
	var charms []*Charm
	iter := charmsCollection.Find(nsLife.notDead()).Iter()
	for iter.Next(&cdoc) {
		ch := newCharm(st, &cdoc)
		charms = append(charms, ch)
	}
	return charms, errors.Trace(iter.Close())
}

// Charm returns the charm with the given URL. Charms pending upload
// to storage and placeholders are never returned.
func (st *State) Charm(curl *charm.URL) (*Charm, error) {
	charms, closer := st.db().GetCollection(charmsC)
	defer closer()

	cdoc := &charmDoc{}
	what := bson.D{
		{"_id", curl.String()},
		{"placeholder", bson.D{{"$ne", true}}},
		{"pendingupload", bson.D{{"$ne", true}}},
	}
	what = append(what, nsLife.notDead()...)
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
	charms, closer := st.db().GetCollection(charmsC)
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
func (st *State) PrepareLocalCharmUpload(curl *charm.URL) (chosenURL *charm.URL, err error) {
	// Perform a few sanity checks first.
	if curl.Schema != "local" {
		return nil, errors.Errorf("expected charm URL with local schema, got %q", curl)
	}
	if curl.Revision < 0 {
		return nil, errors.Errorf("expected charm URL with revision, got %q", curl)
	}

	revisionSeq := charmRevSeqName(curl.WithRevision(-1).String())
	revision, err := sequenceWithMin(st, revisionSeq, curl.Revision)
	if err != nil {
		return nil, errors.Annotate(err, "unable to allocate charm revision")
	}
	allocatedURL := curl.WithRevision(revision)

	ops, err := insertPendingCharmOps(st, allocatedURL)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := st.db().RunTransaction(ops); err != nil {
		return nil, errors.Trace(err)
	}
	return allocatedURL, nil
}

const charmRevSeqPrefix = "charmrev-"

func charmRevSeqName(baseURL string) string {
	return charmRevSeqPrefix + baseURL
}

func isCharmRevSeqName(name string) bool {
	return strings.HasPrefix(name, charmRevSeqPrefix)
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

	charms, closer := st.db().GetCollection(charmsC)
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
				URL:           curl,
				PendingUpload: true,
			}
			return insertAnyCharmOps(st, &uploadedCharm)
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
	if err = st.db().Run(buildTxn); err == nil {
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
// means that if the charm is to be deployed, it will need to first be uploaded to model storage.
func (st *State) AddStoreCharmPlaceholder(curl *charm.URL) (err error) {
	// Perform sanity checks first.
	if curl.Schema != "cs" {
		return errors.Errorf("expected charm URL with cs schema, got %q", curl)
	}
	if curl.Revision < 0 {
		return errors.Errorf("expected charm URL with revision, got %q", curl)
	}
	charms, closer := st.db().GetCollection(charmsC)
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
	return errors.Trace(st.db().Run(buildTxn))
}

// UpdateUploadedCharm marks the given charm URL as uploaded and
// updates the rest of its data, returning it as *state.Charm.
func (st *State) UpdateUploadedCharm(info CharmInfo) (*Charm, error) {
	charms, closer := st.db().GetCollection(charmsC)
	defer closer()

	doc := &charmDoc{}
	err := charms.FindId(info.ID.String()).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("charm %q", info.ID)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !doc.PendingUpload {
		return nil, errors.Trace(&ErrCharmAlreadyUploaded{info.ID})
	}

	ops, err := updateCharmOps(st, info, stillPending)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := st.db().RunTransaction(ops); err != nil {
		return nil, onAbort(err, ErrCharmRevisionAlreadyModified)
	}
	return st.Charm(info.ID)
}
