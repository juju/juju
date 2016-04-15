// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"net/url"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/mongo"
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
	DocID     string     `bson:"_id"`
	URL       *charm.URL `bson:"url"` // DANGEROUS see below
	ModelUUID string     `bson:"model-uuid"`

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
	Macaroon      []byte `bson:"macaroon"`
}

// CharmInfo contains all the data necessary to store a charm's metadata.
type CharmInfo struct {
	Charm       charm.Charm
	ID          *charm.URL
	StoragePath string
	SHA256      string
	Macaroon    macaroon.Slice
}

// insertCharmOps returns the txn operations necessary to insert the supplied
// charm data. If curl is nil, an error will be returned.
func insertCharmOps(st *State, info CharmInfo) ([]txn.Op, error) {
	if info.ID == nil {
		return nil, errors.New("*charm.URL was nil")
	}

	doc := charmDoc{
		DocID:        info.ID.String(),
		URL:          info.ID,
		ModelUUID:    st.ModelTag().Id(),
		Meta:         info.Charm.Meta(),
		Config:       safeConfig(info.Charm),
		Metrics:      info.Charm.Metrics(),
		Actions:      info.Charm.Actions(),
		BundleSha256: info.SHA256,
		StoragePath:  info.StoragePath,
	}
	if info.Macaroon != nil {
		mac, err := info.Macaroon.MarshalBinary()
		if err != nil {
			return nil, errors.Annotate(err, "can't convert macaroon to binary for storage")
		}
		doc.Macaroon = mac
	}
	return insertAnyCharmOps(&doc)
}

// insertPlaceholderCharmOps returns the txn operations necessary to insert a
// charm document referencing a store charm that is not yet directly accessible
// within the model. If curl is nil, an error will be returned.
func insertPlaceholderCharmOps(st *State, curl *charm.URL) ([]txn.Op, error) {
	if curl == nil {
		return nil, errors.New("*charm.URL was nil")
	}
	return insertAnyCharmOps(&charmDoc{
		DocID:       curl.String(),
		URL:         curl,
		ModelUUID:   st.ModelTag().Id(),
		Placeholder: true,
	})
}

// insertPendingCharmOps returns the txn operations necessary to insert a charm
// document referencing a charm that has yet to be uploaded to the model.
// If curl is nil, an error will be returned.
func insertPendingCharmOps(st *State, curl *charm.URL) ([]txn.Op, error) {
	if curl == nil {
		return nil, errors.New("*charm.URL was nil")
	}
	return insertAnyCharmOps(&charmDoc{
		DocID:         curl.String(),
		URL:           curl,
		ModelUUID:     st.ModelTag().Id(),
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
	st *State, info CharmInfo, assert interface{},
) ([]txn.Op, error) {

	data := bson.D{
		{"meta", info.Charm.Meta()},
		{"config", safeConfig(info.Charm)},
		{"actions", info.Charm.Actions()},
		{"metrics", info.Charm.Metrics()},
		{"storagepath", info.StoragePath},
		{"bundlesha256", info.SHA256},
		{"pendingupload", false},
		{"placeholder", false},
	}

	if len(info.Macaroon) > 0 {
		mac, err := info.Macaroon.MarshalBinary()
		if err != nil {
			return nil, errors.Annotate(err, "can't convert macaroon to binary for storage")
		}
		data = append(data, bson.DocElem{"macaroon", mac})
	}

	updateFields := bson.D{{"$set", data}}
	return []txn.Op{{
		C:      charmsC,
		Id:     info.ID.String(),
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
			unescapedName := unescapeReplacer.Replace(optionName)
			unescapedConfig.Options[unescapedName] = option
		}
		cdoc.Config = unescapedConfig
	}
	ch := Charm{st: st, doc: *cdoc}
	return &ch
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
// charm archives to model storage.
func (c *Charm) BundleURL() *url.URL {
	return c.doc.BundleURL
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
	ops, err := updateCharmOps(c.st, info, txn.DocExists)
	if err != nil {
		return errors.Trace(err)
	}
	if err := c.st.runTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}
