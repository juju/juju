// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v4"
	jujutxn "github.com/juju/txn/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/mongo/utils"
	"github.com/juju/juju/state/watcher"
)

// LabelExists is returned when a duplicate label is used.
const LabelExists = errors.ConstError("label exists")

// CreateSecretParams are used to create a secret.
type CreateSecretParams struct {
	UpdateSecretParams

	Version int
	Owner   names.Tag
}

// UpdateSecretParams are used to update a secret.
type UpdateSecretParams struct {
	LeaderToken    leadership.Token
	RotatePolicy   *secrets.RotatePolicy
	NextRotateTime *time.Time
	ExpireTime     *time.Time
	Description    *string
	Label          *string
	Params         map[string]interface{}
	Data           secrets.SecretData
	ValueRef       *secrets.ValueRef
	AutoPrune      *bool
}

func (u *UpdateSecretParams) hasUpdate() bool {
	return u.NextRotateTime != nil ||
		u.RotatePolicy != nil ||
		u.Description != nil ||
		u.Label != nil ||
		u.ExpireTime != nil ||
		len(u.Data) > 0 ||
		u.ValueRef != nil ||
		len(u.Params) > 0 ||
		u.AutoPrune != nil
}

// ChangeSecretBackendParams are used to change the backend of a secret.
type ChangeSecretBackendParams struct {
	Token    leadership.Token
	URI      *secrets.URI
	Revision int
	ValueRef *secrets.ValueRef
	Data     secrets.SecretData
}

// SecretsFilter holds attributes to match when listing secrets.
type SecretsFilter struct {
	URI          *secrets.URI
	Label        *string
	OwnerTags    []names.Tag
	ConsumerTags []names.Tag
}

// SecretsStore instances use mongo as a secrets store.
type SecretsStore interface {
	CreateSecret(*secrets.URI, CreateSecretParams) (*secrets.SecretMetadata, error)
	UpdateSecret(*secrets.URI, UpdateSecretParams) (*secrets.SecretMetadata, error)
	DeleteSecret(*secrets.URI, ...int) ([]secrets.ValueRef, error)
	GetSecret(*secrets.URI) (*secrets.SecretMetadata, error)
	GetSecretValue(*secrets.URI, int) (secrets.SecretValue, *secrets.ValueRef, error)
	ListSecrets(SecretsFilter) ([]*secrets.SecretMetadata, error)
	ListModelSecrets(bool) (map[string]set.Strings, error)
	ListSecretRevisions(uri *secrets.URI) ([]*secrets.SecretRevisionMetadata, error)
	ListUnusedSecretRevisions(uri *secrets.URI) ([]int, error)
	GetSecretRevision(uri *secrets.URI, revision int) (*secrets.SecretRevisionMetadata, error)
	WatchObsolete(owners []names.Tag) (StringsWatcher, error)
	WatchRevisionsToPrune(ownerTags []names.Tag) (StringsWatcher, error)
	ChangeSecretBackend(ChangeSecretBackendParams) error
}

// NewSecrets creates a new mongo backed secrets store.
func NewSecrets(st *State) *secretsStore {
	return &secretsStore{st: st}
}

type secretMetadataDoc struct {
	DocID string `bson:"_id"`

	Version  int    `bson:"version"`
	OwnerTag string `bson:"owner-tag"`

	Description string `bson:"description"`
	Label       string `bson:"label"`

	// LatestRevision is denormalised here - it is the
	// revision of the latest revision doc,
	LatestRevision int `bson:"latest-revision"`
	// LatestExpireTime is denormalised here - it is the
	// expire time of the latest revision doc,
	LatestExpireTime *time.Time `bson:"latest-expire-time"`

	RotatePolicy string `bson:"rotate-policy"`

	CreateTime time.Time `bson:"create-time"`
	UpdateTime time.Time `bson:"update-time"`

	// AutoPrune is true if the secret revisions should be pruned when it's not been used.
	AutoPrune bool `bson:"auto-prune"`
}

type valueRefDoc struct {
	BackendID  string `bson:"backend-id"`
	RevisionID string `bson:"revision-id"`
}

type secretRevisionDoc struct {
	DocID    string `bson:"_id"`
	TxnRevno int64  `bson:"txn-revno"`

	Revision   int            `bson:"revision"`
	CreateTime time.Time      `bson:"create-time"`
	UpdateTime time.Time      `bson:"update-time"`
	ExpireTime *time.Time     `bson:"expire-time,omitempty"`
	Obsolete   bool           `bson:"obsolete"`
	Data       secretsDataMap `bson:"data"`
	ValueRef   *valueRefDoc   `bson:"value-reference,omitempty"`

	// PendingDelete is true if the revision is to be deleted.
	// It will not be drained to a new active backend.
	PendingDelete bool `bson:"pending-delete"`

	// OwnerTag is denormalised here so that watchers do not need
	// to do an extra query on the secret metadata collection to
	// filter on owner.
	OwnerTag string `bson:"owner-tag"`
}

type secretsDataMap map[string]interface{}

func (m *secretsDataMap) SetBSON(raw bson.Raw) error {
	rawMap := make(map[string]interface{})
	if err := raw.Unmarshal(rawMap); err != nil {
		return err
	}
	*m = utils.UnescapeKeys(rawMap)
	return nil
}

func (m secretsDataMap) GetBSON() (interface{}, error) {
	escapedMap := utils.EscapeKeys(m)
	return escapedMap, nil
}

type secretsStore struct {
	st *State
}

func ptr[T any](v T) *T {
	return &v
}

func toValue[T any](v *T) T {
	if v == nil {
		return *new(T)
	}
	return *v
}

func (s *secretsStore) secretMetadataDoc(uri *secrets.URI, p *CreateSecretParams) (*secretMetadataDoc, error) {
	now := s.st.nowToTheSecond()
	md := &secretMetadataDoc{
		DocID:      uri.ID,
		Version:    p.Version,
		OwnerTag:   p.Owner.String(),
		CreateTime: now,
		UpdateTime: now,
	}
	_, err := names.ParseTag(md.OwnerTag)
	if err != nil {
		return nil, errors.Annotate(err, "invalid owner tag")
	}
	err = s.updateSecretMetadataDoc(md, &p.UpdateSecretParams)
	return md, err
}

func (s *secretsStore) updateSecretMetadataDoc(doc *secretMetadataDoc, p *UpdateSecretParams) error {
	if p.Description != nil {
		doc.Description = toValue(p.Description)
	}
	if p.Label != nil {
		doc.Label = toValue(p.Label)
	}
	if p.AutoPrune != nil {
		doc.AutoPrune = *p.AutoPrune
	}
	if p.RotatePolicy != nil {
		doc.RotatePolicy = string(toValue(p.RotatePolicy))
	}
	hasData := len(p.Data) > 0 || p.ValueRef != nil
	if p.ExpireTime != nil || hasData {
		if p.ExpireTime == nil || p.ExpireTime.IsZero() {
			doc.LatestExpireTime = nil
		} else {
			doc.LatestExpireTime = ptr(toValue(p.ExpireTime).Round(time.Second).UTC())
		}
	}
	if hasData {
		doc.LatestRevision++
	}
	doc.UpdateTime = s.st.nowToTheSecond()
	return nil
}

func secretRevisionKey(uri *secrets.URI, revision int) string {
	return fmt.Sprintf("%s/%d", uri.ID, revision)
}

func splitSecretRevision(c string) (string, int) {
	parts := strings.Split(c, "/")
	if len(parts) < 2 {
		return parts[0], 0
	}
	rev, _ := strconv.Atoi(parts[1])
	return parts[0], rev
}

func (s *secretsStore) secretRevisionDoc(uri *secrets.URI, owner string, revision int, expireTime *time.Time, data secrets.SecretData, valueRef *secrets.ValueRef) *secretRevisionDoc {
	dataCopy := make(secretsDataMap)
	for k, v := range data {
		dataCopy[k] = v
	}
	now := s.st.nowToTheSecond()
	var valRefDoc *valueRefDoc
	if valueRef != nil {
		valRefDoc = &valueRefDoc{
			BackendID:  valueRef.BackendID,
			RevisionID: valueRef.RevisionID,
		}
	}
	doc := &secretRevisionDoc{
		DocID:      secretRevisionKey(uri, revision),
		Revision:   revision,
		OwnerTag:   owner,
		CreateTime: now,
		UpdateTime: now,
		Data:       dataCopy,
		ValueRef:   valRefDoc,
	}
	if expireTime != nil {
		expire := expireTime.Round(time.Second).UTC()
		doc.ExpireTime = &expire
	}
	return doc
}

// CreateSecret creates a new secret.
func (s *secretsStore) CreateSecret(uri *secrets.URI, p CreateSecretParams) (*secrets.SecretMetadata, error) {
	if len(p.Data) == 0 && p.ValueRef == nil {
		return nil, errors.New("cannot create a secret without content")
	}
	metadataDoc, err := s.secretMetadataDoc(uri, &p)
	if err != nil {
		return nil, errors.Trace(err)
	}
	revision := 1
	valueDoc := s.secretRevisionDoc(uri, p.Owner.String(), revision, p.ExpireTime, p.Data, p.ValueRef)
	// OwnerTag has already been validated.
	owner, _ := names.ParseTag(metadataDoc.OwnerTag)
	entity, scopeCollName, scopeDocID, err := s.st.findSecretEntity(owner)
	if err != nil {
		return nil, errors.Annotate(err, "invalid owner reference")
	}
	if entity.Life() != Alive {
		return nil, errors.Errorf("cannot create secret for owner %q which is not alive", owner)
	}
	isOwnerAliveOp := txn.Op{
		C:      scopeCollName,
		Id:     scopeDocID,
		Assert: isAliveDoc,
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		var ops []txn.Op
		if p.Label != nil {
			uniqueLabelOps, err := s.st.uniqueSecretOwnerLabelOps(owner, *p.Label)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, uniqueLabelOps...)
		}
		if attempt > 0 {
			if _, _, err := s.getSecretValue(uri, revision, false); err == nil {
				return nil, errors.AlreadyExistsf("secret value for %q", uri.String())
			}
		}
		ops = append(ops, []txn.Op{
			{
				C:      secretMetadataC,
				Id:     metadataDoc.DocID,
				Assert: txn.DocMissing,
				Insert: *metadataDoc,
			}, {
				C:      secretRevisionsC,
				Id:     valueDoc.DocID,
				Assert: txn.DocMissing,
				Insert: *valueDoc,
			}, isOwnerAliveOp,
		}...)
		if valueDoc.ValueRef != nil {
			refOps, err := s.st.incBackendRevisionCountOps(valueDoc.ValueRef.BackendID, 1)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, refOps...)
		}
		if p.NextRotateTime != nil {
			rotateOps, err := s.secretRotationOps(uri, metadataDoc.OwnerTag, p.RotatePolicy, p.NextRotateTime)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, rotateOps...)
		}
		return ops, nil
	}
	err = s.st.db().Run(buildTxnWithLeadership(buildTxn, p.LeaderToken))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.toSecretMetadata(metadataDoc, p.NextRotateTime)
}

func (st *State) checkExists(uri *secrets.URI) error {
	secretMetadataCollection, closer := st.db().GetCollection(secretMetadataC)
	defer closer()
	n, err := secretMetadataCollection.FindId(uri.ID).Count()
	if err != nil {
		return errors.Trace(err)
	}
	if n == 0 {
		return errors.NotFoundf("secret %q", uri.String())
	}
	return nil
}

// UpdateSecret updates an existing secret.
func (s *secretsStore) UpdateSecret(uri *secrets.URI, p UpdateSecretParams) (*secrets.SecretMetadata, error) {
	if !p.hasUpdate() {
		return nil, errors.New("must specify a new value or metadata to update a secret")
	}
	// Used later but look up early and return if it fails.
	nextRotateTime, err := s.st.nextRotateTime(uri.ID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	secretMetadataCollection, closer := s.st.db().GetCollection(secretMetadataC)
	defer closer()

	// Pre-process the expire time update.
	haveExpireTime := false
	newExpireTime := p.ExpireTime
	if newExpireTime != nil {
		haveExpireTime = true
		if newExpireTime.IsZero() {
			newExpireTime = nil
		}
	}

	var metadataDoc secretMetadataDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		err := secretMetadataCollection.FindId(uri.ID).One(&metadataDoc)
		if err == mgo.ErrNotFound {
			return nil, errors.NotFoundf("secret %q", uri.String())
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		var ops []txn.Op
		if p.Label != nil && *p.Label != metadataDoc.Label {
			// OwnerTag has already been validated.
			owner, _ := names.ParseTag(metadataDoc.OwnerTag)
			uniqueLabelOps, err := s.st.uniqueSecretOwnerLabelOps(owner, *p.Label)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, uniqueLabelOps...)
		}
		currentRevision := metadataDoc.LatestRevision
		if err := s.updateSecretMetadataDoc(&metadataDoc, &p); err != nil {
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		ops = append(ops, []txn.Op{
			{
				C:      secretMetadataC,
				Id:     metadataDoc.DocID,
				Assert: bson.D{{"latest-revision", currentRevision}},
				Update: bson.M{"$set": metadataDoc},
			},
		}...)
		_, _, err = s.getSecretValue(uri, metadataDoc.LatestRevision, false)
		revisionExists := err == nil
		if !revisionExists && !errors.Is(err, errors.NotFound) {
			return nil, errors.Trace(err)
		}
		if len(p.Data) > 0 || p.ValueRef != nil {
			if revisionExists {
				return nil, errors.AlreadyExistsf("secret value with revision %d for %q", metadataDoc.LatestRevision, uri.String())
			}
			revisionDoc := s.secretRevisionDoc(uri, metadataDoc.OwnerTag, metadataDoc.LatestRevision, newExpireTime, p.Data, p.ValueRef)
			ops = append(ops, txn.Op{
				C:      secretRevisionsC,
				Id:     revisionDoc.DocID,
				Assert: txn.DocMissing,
				Insert: *revisionDoc,
			})
			if p.ValueRef != nil {
				refOps, err := s.st.incBackendRevisionCountOps(p.ValueRef.BackendID, 1)
				if err != nil {
					return nil, errors.Trace(err)
				}
				ops = append(ops, refOps...)
			}
			// Ensure no new consumers are added while update is in progress.
			countOps, err := s.st.checkConsumerCountOps(uri, 0)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, countOps...)

			updateConsumersOps, err := s.st.secretUpdateConsumersOps(secretConsumersC, uri, metadataDoc.LatestRevision)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, updateConsumersOps...)

			updateRemoteConsumersOps, err := s.st.secretUpdateConsumersOps(secretRemoteConsumersC, uri, metadataDoc.LatestRevision)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, updateRemoteConsumersOps...)

			// Saving a new revision might result in the previous latest revision
			// being obsolete if it had not been read yet.
			obsoleteOps, err := s.st.markObsoleteRevisionOps(uri, "", revisionDoc.Revision)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, obsoleteOps...)

		} else if haveExpireTime {
			if !revisionExists {
				return nil, errors.NotFoundf("reversion %d for secret %q", metadataDoc.LatestRevision, uri.String())
			}
			// If the expire time is being removed, it needs to be unset.
			toSet := bson.D{{"update-time", s.st.nowToTheSecond()}}
			if newExpireTime != nil {
				toSet = append(toSet, bson.DocElem{"expire-time", newExpireTime})
			}
			var toUnset bson.D
			if newExpireTime == nil {
				toUnset = bson.D{{"expire-time", nil}}
			}
			updates := bson.D{{"$set", toSet}}
			if len(toUnset) > 0 {
				updates = append(updates, bson.DocElem{"$unset", toUnset})
			}
			ops = append(ops, txn.Op{
				C:      secretRevisionsC,
				Id:     secretRevisionKey(uri, metadataDoc.LatestRevision),
				Assert: txn.DocExists,
				Update: updates,
			})
		}
		if p.RotatePolicy != nil || p.NextRotateTime != nil {
			rotateOps, err := s.secretRotationOps(uri, metadataDoc.OwnerTag, p.RotatePolicy, p.NextRotateTime)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, rotateOps...)
		}
		return ops, nil
	}
	err = s.st.db().Run(buildTxnWithLeadership(buildTxn, p.LeaderToken))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.toSecretMetadata(&metadataDoc, nextRotateTime)
}

func (st *State) nextRotateTime(docID string) (*time.Time, error) {
	secretRotateCollection, closer := st.db().GetCollection(secretRotateC)
	defer closer()

	var rotateDoc secretRotationDoc
	err := secretRotateCollection.FindId(docID).One(&rotateDoc)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &rotateDoc.NextRotateTime, nil
}

func (s *secretsStore) toSecretMetadata(doc *secretMetadataDoc, nextRotateTime *time.Time) (*secrets.SecretMetadata, error) {
	uri, err := secrets.ParseURI(s.st.localID(doc.DocID))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &secrets.SecretMetadata{
		URI:              uri,
		Version:          doc.Version,
		RotatePolicy:     secrets.RotatePolicy(doc.RotatePolicy),
		NextRotateTime:   nextRotateTime,
		LatestRevision:   doc.LatestRevision,
		LatestExpireTime: doc.LatestExpireTime,
		Description:      doc.Description,
		Label:            doc.Label,
		OwnerTag:         doc.OwnerTag,
		AutoPrune:        doc.AutoPrune,
		CreateTime:       doc.CreateTime,
		UpdateTime:       doc.UpdateTime,
	}, nil
}

// DeleteSecret deletes the specified secret revisions.
// If revisions is nil or the last remaining revisions are
// removed, the entire secret is deleted and the return bool is true.
// Also returned are any references to content stored in an external
// backend for any deleted revisions.
func (s *secretsStore) DeleteSecret(uri *secrets.URI, revisions ...int) (external []secrets.ValueRef, err error) {
	return s.st.deleteSecrets([]*secrets.URI{uri}, revisions...)
}

func (st *State) deleteSecrets(uris []*secrets.URI, revisions ...int) (external []secrets.ValueRef, err error) {
	// We will bulk delete the various artefacts, starting with the secret itself.
	// Deleting the parent secret metadata first will ensure that any consumers of
	// the secret get notified and subsequent attempts to access any secret
	// attributes (revision etc) return not found.
	// It is not practical to do this record by record in a legacy client side mgo txn operation.
	if len(uris) == 0 && len(revisions) == 0 {
		// Nothing to remove.
		return nil, nil
	}

	if len(uris) == 0 || len(uris) > 1 && len(revisions) > 0 {
		return nil, errors.Errorf("PROGRAMMING ERROR: invalid secret deletion args uris=%v, revisions=%v", uris, revisions)
	}
	session := st.MongoSession()
	err = session.StartTransaction()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		if err == nil {
			err = session.CommitTransaction()
			return
		}
		if err2 := session.AbortTransaction(); err2 != nil {
			logger.Warningf("aborting failed delete select transaction: %v", err2)
		}
	}()

	// If we're not deleting all revisions for a secret, just remove the affected
	// revision docs and exit early.
	if len(revisions) > 0 {
		uri := uris[0]

		secretRevisionsCollection, closer := st.db().GetCollection(secretRevisionsC)
		defer closer()

		var savedRevisionDocs []secretRevisionDoc
		err := secretRevisionsCollection.Find(bson.D{{"_id",
			bson.D{{"$regex", fmt.Sprintf("%s/.*", uri.ID)}}}}).Select(
			bson.D{{"revision", 1}, {"value-reference", 1}}).All(&savedRevisionDocs)
		if err != nil {
			return nil, errors.Annotatef(err, "counting revisions for %s", uri.String())
		}
		externalRevisionCounts := make(map[string]int)
		toDelete := set.NewInts(revisions...)
		savedRevisions := set.NewInts()
		for _, r := range savedRevisionDocs {
			savedRevisions.Add(r.Revision)
			if !toDelete.Contains(r.Revision) {
				continue
			}
			if r.ValueRef != nil {
				external = append(external, secrets.ValueRef{
					BackendID:  r.ValueRef.BackendID,
					RevisionID: r.ValueRef.RevisionID,
				})
				externalRevisionCounts[r.ValueRef.BackendID] = externalRevisionCounts[r.ValueRef.BackendID] + 1
			}
		}
		if savedRevisions.Difference(toDelete).Size() > 0 {
			revs := make([]string, len(revisions))
			for i, r := range revisions {
				revs[i] = strconv.Itoa(r)
			}
			revisionRegexp := fmt.Sprintf("(%s)", strings.Join(revs, "|"))
			_, err = secretRevisionsCollection.Writeable().RemoveAll(bson.D{{
				"_id", bson.D{{"$regex", fmt.Sprintf("%s/%s", uri.ID, revisionRegexp)}},
			}})
			if err != nil {
				return nil, errors.Annotatef(err, "deleting revisions for %s", uri.String())
			}
			// Decrement the count of secret revisions stored in the external backends.
			// This allows backends without stored revisions to be removed without using force.
			globalRefCountsCollection, closer := st.db().GetCollection(globalRefcountsC)
			defer closer()
			for backendID, count := range externalRevisionCounts {
				if secrets.IsInternalSecretBackendID(backendID) {
					continue
				}
				err = globalRefCountsCollection.Writeable().UpdateId(
					secretBackendRefCountKey(backendID),
					bson.D{{"$inc", bson.D{{"refcount", -1 * count}}}})
				if err != nil {
					return nil, errors.Annotatef(err, "updating backend refcounts for %s", uri.String())
				}
			}
			return nil, nil
		}
	}

	for _, uri := range uris {
		deletedExternal, err := st.deleteOne(uri)
		if err != nil {
			return nil, errors.Annotatef(err, "deleting secret %q", uri.String())
		}
		// Don't collate the external revisions twice.
		// If specific revisions are being removed, the external
		// references have already been added.
		if len(revisions) == 0 {
			external = append(external, deletedExternal...)
		}
	}
	return external, nil
}

func (st *State) deleteOne(uri *secrets.URI) (external []secrets.ValueRef, _ error) {
	secretMetadataCollection, closer := st.db().GetCollection(secretMetadataC)
	defer closer()

	secretRevisionsCollection, closer := st.db().GetCollection(secretRevisionsC)
	defer closer()

	var md secretMetadataDoc
	err := secretMetadataCollection.FindId(uri.ID).One(&md)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	_, err = secretMetadataCollection.Writeable().RemoveAll(bson.D{{
		"_id", uri.ID,
	}})
	if err != nil {
		return nil, errors.Annotatef(err, "deleting revisions for %s", uri.String())
	}

	secretRotateCollection, closer := st.db().GetCollection(secretRotateC)
	defer closer()
	_, err = secretRotateCollection.Writeable().RemoveAll(bson.D{{
		"_id", uri.ID,
	}})
	if err != nil {
		return nil, errors.Annotatef(err, "deleting revisions for %s", uri.String())
	}

	var savedRevisionDocs []secretRevisionDoc
	externalRevisionCounts := make(map[string]int)
	err = secretRevisionsCollection.Find(bson.D{{"_id",
		bson.D{{"$regex", fmt.Sprintf("%s/.*", uri.ID)}}}}).Select(
		bson.D{{"revision", 1}, {"value-reference", 1}}).All(&savedRevisionDocs)
	if err != nil {
		return nil, errors.Annotatef(err, "reading revisions for %s", uri.String())
	}
	for _, r := range savedRevisionDocs {
		if r.ValueRef != nil {
			external = append(external, secrets.ValueRef{
				BackendID:  r.ValueRef.BackendID,
				RevisionID: r.ValueRef.RevisionID,
			})
			externalRevisionCounts[r.ValueRef.BackendID] = externalRevisionCounts[r.ValueRef.BackendID] + 1
		}
	}
	_, err = secretRevisionsCollection.Writeable().RemoveAll(bson.D{{
		"_id", bson.D{{"$regex", fmt.Sprintf("%s/.*", uri.ID)}},
	}})
	if err != nil {
		return nil, errors.Annotatef(err, "deleting revisions for %s", uri.String())
	}

	secretPermissionsCollection, closer := st.db().GetCollection(secretPermissionsC)
	defer closer()
	_, err = secretPermissionsCollection.Writeable().RemoveAll(bson.D{{
		"_id", bson.D{{"$regex", fmt.Sprintf("%s#.*", uri.ID)}},
	}})
	if err != nil {
		return nil, errors.Annotatef(err, "deleting permissions for %s", uri.String())
	}

	if err = st.removeSecretConsumerInfo(uri); err != nil {
		return nil, errors.Trace(err)
	}
	if err = st.removeSecretRemoteConsumerInfo(uri); err != nil {
		return nil, errors.Trace(err)
	}

	refCountsCollection, closer := st.db().GetCollection(refcountsC)
	defer closer()
	_, err = refCountsCollection.Writeable().RemoveAll(bson.D{{
		"_id", fmt.Sprintf("%s#%s", uri.ID, "consumer"),
	}})
	if err != nil {
		return nil, errors.Annotatef(err, "deleting consumer refcounts for %s", uri.String())
	}

	// Decrement the count of secret revisions stored in the external backends.
	// This allows backends without stored revisions to be removed without using force.
	globalRefCountsCollection, closer := st.db().GetCollection(globalRefcountsC)
	defer closer()
	for backendID, count := range externalRevisionCounts {
		if secrets.IsInternalSecretBackendID(backendID) {
			continue
		}
		err = globalRefCountsCollection.Writeable().UpdateId(
			secretBackendRefCountKey(backendID),
			bson.D{{"$inc", bson.D{{"refcount", -1 * count}}}})
		if err != nil {
			return nil, errors.Annotatef(err, "updating backend refcounts for %s", uri.String())
		}
	}

	if md.Label != "" {
		owner, _ := names.ParseTag(md.OwnerTag)
		_, err = refCountsCollection.Writeable().RemoveAll(bson.D{{
			"_id", secretOwnerLabelKey(owner, md.Label),
		}})
		if err != nil {
			return nil, errors.Annotatef(err, "deleting owner label refcounts for %s", uri.String())
		}
	}
	return external, nil
}

// GetSecretValue gets the secret value for the specified URL.
func (s *secretsStore) GetSecretValue(uri *secrets.URI, revision int) (secrets.SecretValue, *secrets.ValueRef, error) {
	return s.getSecretValue(uri, revision, true)
}

func (s *secretsStore) getSecretValue(uri *secrets.URI, revision int, checkExists bool) (secrets.SecretValue, *secrets.ValueRef, error) {
	if checkExists {
		if err := s.st.checkExists(uri); err != nil {
			return nil, nil, errors.Trace(err)
		}
	}
	secretValuesCollection, closer := s.st.db().GetCollection(secretRevisionsC)
	defer closer()

	var doc secretRevisionDoc
	key := secretRevisionKey(uri, revision)
	err := secretValuesCollection.FindId(key).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, nil, errors.NotFoundf("secret revision %q", key)
	}
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	data := make(secrets.SecretData)
	for k, v := range doc.Data {
		data[k] = fmt.Sprintf("%v", v)
	}
	var valueRef *secrets.ValueRef
	if doc.ValueRef != nil {
		valueRef = &secrets.ValueRef{
			BackendID:  doc.ValueRef.BackendID,
			RevisionID: doc.ValueRef.RevisionID,
		}
	}
	return secrets.NewSecretValue(data), valueRef, nil
}

// ChangeSecretBackend updates the backend ID for the provided secret revision.
func (s *secretsStore) ChangeSecretBackend(arg ChangeSecretBackendParams) error {
	if err := s.st.checkExists(arg.URI); err != nil {
		return errors.Trace(err)
	}

	secretRevisionsCollection, closer := s.st.db().GetCollection(secretRevisionsC)
	defer closer()
	var doc secretRevisionDoc
	key := secretRevisionKey(arg.URI, arg.Revision)
	err := secretRevisionsCollection.FindId(key).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("secret revision %q", key)
	}
	if err != nil {
		return errors.Trace(err)
	}

	dataCopy := make(secretsDataMap)
	for k, v := range arg.Data {
		dataCopy[k] = v
	}
	var valRefDoc *valueRefDoc
	if arg.ValueRef != nil {
		valRefDoc = &valueRefDoc{
			BackendID:  arg.ValueRef.BackendID,
			RevisionID: arg.ValueRef.RevisionID,
		}
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		var ops []txn.Op
		if doc.ValueRef != nil {
			refOps, err := s.st.decSecretBackendRefCountOp(doc.ValueRef.BackendID)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, refOps...)
		}
		if valRefDoc != nil {
			refOps, err := s.st.incBackendRevisionCountOps(valRefDoc.BackendID, 1)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, refOps...)
		}
		return append(ops, txn.Op{
			C:      secretRevisionsC,
			Id:     doc.DocID,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"value-reference": valRefDoc, "data": dataCopy}},
		}), nil
	}
	err = s.st.db().Run(buildTxnWithLeadership(buildTxn, arg.Token))
	return errors.Trace(err)
}

// GetSecret gets the secret metadata for the specified URL.
func (s *secretsStore) GetSecret(uri *secrets.URI) (*secrets.SecretMetadata, error) {
	if uri == nil {
		return nil, errors.NewNotValid(nil, "empty URI")
	}

	secretMetadataCollection, closer := s.st.db().GetCollection(secretMetadataC)
	defer closer()

	var doc secretMetadataDoc
	err := secretMetadataCollection.FindId(uri.ID).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("secret %q", uri.String())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	nextRotateTime, err := s.st.nextRotateTime(uri.ID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.toSecretMetadata(&doc, nextRotateTime)
}

func secretOwnerTerm(owners []string) bson.DocElem {
	return bson.DocElem{Name: "owner-tag", Value: bson.D{{Name: "$in", Value: owners}}}
}

// ListSecrets list the secrets using the specified filter.
func (s *secretsStore) ListSecrets(filter SecretsFilter) ([]*secrets.SecretMetadata, error) {
	secretMetadataCollection, closer := s.st.db().GetCollection(secretMetadataC)
	defer closer()

	var docs []secretMetadataDoc
	q := bson.D{}
	if filter.URI != nil {
		q = append(q, bson.DocElem{"_id", filter.URI.ID})
	}
	if filter.Label != nil {
		q = append(q, bson.DocElem{"label", *filter.Label})
	}
	if len(filter.OwnerTags) > 0 {
		owners := make([]string, len(filter.OwnerTags))
		for i, tag := range filter.OwnerTags {
			owners[i] = tag.String()
		}
		q = append(q, secretOwnerTerm(owners))
	}
	// Only query here if we want everything or no consumers were specified.
	// We need to do the consumer processing below as the results are seeded
	// from a different collection.
	if len(q) > 0 || len(filter.ConsumerTags) == 0 {
		err := secretMetadataCollection.Find(q).All(&docs)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	result := make([]*secrets.SecretMetadata, len(docs))
	for i, doc := range docs {
		nextRotateTime, err := s.st.nextRotateTime(doc.DocID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result[i], err = s.toSecretMetadata(&doc, nextRotateTime)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	if len(filter.ConsumerTags) == 0 {
		return result, nil
	}
	consumers := make([]string, len(filter.ConsumerTags))
	for i, tag := range filter.ConsumerTags {
		consumers[i] = tag.String()
	}
	consumedIds, err := s.listConsumedSecrets(consumers)
	if err != nil {
		return nil, errors.Trace(err)
	}

	docs = []secretMetadataDoc(nil)
	q2 := bson.M{"_id": bson.M{"$in": consumedIds}}
	err = secretMetadataCollection.Find(q2).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, doc := range docs {
		nextRotateTime, err := s.st.nextRotateTime(doc.DocID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		md, err := s.toSecretMetadata(&doc, nextRotateTime)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result = append(result, md)
	}
	return result, nil
}

// allModelRevisions uses a raw collection to load secret revisions for all models.
func (s *secretsStore) allModelRevisions() ([]secretRevisionDoc, error) {
	var docs []secretRevisionDoc
	secretRevisionCollection, closer := s.st.db().GetRawCollection(secretRevisionsC)
	defer closer()

	err := secretRevisionCollection.Find(nil).Select(bson.D{{"_id", 1}, {"value-reference", 1}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

// modelRevisions uses a warpped collection to load secret revisions for the current model.
func (s *secretsStore) modelRevisions() ([]secretRevisionDoc, error) {
	var docs []secretRevisionDoc
	secretRevisionCollection, closer := s.st.db().GetCollection(secretRevisionsC)
	defer closer()

	err := secretRevisionCollection.Find(nil).Select(bson.D{{"_id", 1}, {"value-reference", 1}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

// ListModelSecrets returns a map of backend id to secret uris stored in that backend.
// If all is true, secrets for all models are included, else just the current model.
func (s *secretsStore) ListModelSecrets(all bool) (map[string]set.Strings, error) {
	var (
		docs []secretRevisionDoc
		err  error
	)
	if all {
		docs, err = s.allModelRevisions()
	} else {
		docs, err = s.modelRevisions()
	}
	if err != nil {
		return nil, errors.Annotate(err, "reading secret revisions")
	}

	controllerUUID := s.st.ControllerUUID()
	result := make(map[string]set.Strings)
	for _, doc := range docs {
		// Deal with the raw doc id.
		parts := strings.SplitN(doc.DocID, ":", 2)
		if len(parts) < 2 {
			continue
		}
		uriStr, _ := splitSecretRevision(parts[1])
		backendID := controllerUUID
		if doc.ValueRef != nil {
			backendID = doc.ValueRef.BackendID
		}
		if _, ok := result[backendID]; !ok {
			result[backendID] = set.NewStrings(uriStr)
			continue
		}
		result[backendID].Add(uriStr)
	}
	return result, nil
}

func (s *secretsStore) listConsumedSecrets(consumers []string) ([]string, error) {
	secretPermissionsCollection, closer := s.st.db().GetCollection(secretPermissionsC)
	defer closer()
	var docs []secretPermissionDoc
	err := secretPermissionsCollection.Find(bson.M{
		"subject-tag": bson.M{
			"$in": consumers,
		},
		"role": secrets.RoleView,
	}).Select(bson.D{{"_id", 1}}).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "reading permissions for %q", consumers)
	}
	var ids []string
	for _, doc := range docs {
		id := s.st.localID(doc.DocID)
		ids = append(ids, strings.Split(id, "#")[0])
	}
	return ids, nil
}

// allSecretPermissions is used for model export.
func (s *secretsStore) allSecretPermissions() ([]secretPermissionDoc, error) {
	secretPermissionCollection, closer := s.st.db().GetCollection(secretPermissionsC)
	defer closer()

	var docs []secretPermissionDoc

	err := secretPermissionCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

// ListSecretRevisions returns the revision metadata for the given secret.
func (s *secretsStore) ListSecretRevisions(uri *secrets.URI) ([]*secrets.SecretRevisionMetadata, error) {
	return s.listSecretRevisions(uri, nil)
}

// ListUnusedSecretRevisions returns the revision numbers that are not consumered by any applications.
func (s *secretsStore) ListUnusedSecretRevisions(uri *secrets.URI) ([]int, error) {
	docs, err := s.listSecretRevisionDocs(uri, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var revisions []int
	for _, doc := range docs {
		if doc.Obsolete {
			revisions = append(revisions, doc.Revision)
		}
	}
	return revisions, nil
}

// GetSecretRevision returns the specified revision metadata for the given secret.
func (s *secretsStore) GetSecretRevision(uri *secrets.URI, revision int) (*secrets.SecretRevisionMetadata, error) {
	rev, err := s.listSecretRevisions(uri, &revision)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(rev) == 0 {
		return nil, errors.NotFoundf("revison %d for secret %q", revision, uri)
	}
	return rev[0], nil
}

func (s *secretsStore) listSecretRevisionDocs(uri *secrets.URI, revision *int) ([]secretRevisionDoc, error) {
	secretRevisionCollection, closer := s.st.db().GetCollection(secretRevisionsC)
	defer closer()

	var (
		docs []secretRevisionDoc
		q    interface{}
	)

	if revision == nil {
		q = bson.D{{"_id", bson.D{{"$regex", uri.ID + "/.*"}}}}
	} else {
		q = bson.D{{"_id", secretRevisionKey(uri, *revision)}}
	}
	err := secretRevisionCollection.Find(q).Sort("_id").All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

func (s *secretsStore) listSecretRevisions(uri *secrets.URI, revision *int) ([]*secrets.SecretRevisionMetadata, error) {
	docs, err := s.listSecretRevisionDocs(uri, revision)
	if err != nil {
		return nil, errors.Trace(err)
	}

	secretBackendsColl, closer := s.st.db().GetCollection(secretBackendsC)
	defer closer()

	backendNames := make(map[string]string)
	result := make([]*secrets.SecretRevisionMetadata, len(docs))
	for i, doc := range docs {
		var (
			valueRef    *secrets.ValueRef
			backendName *string
		)
		if doc.ValueRef != nil {
			valueRef = &secrets.ValueRef{
				BackendID:  doc.ValueRef.BackendID,
				RevisionID: doc.ValueRef.RevisionID,
			}
			if doc.ValueRef.BackendID != s.st.ModelUUID() {
				name, ok := backendNames[doc.ValueRef.BackendID]
				if !ok {
					var backendDoc secretBackendDoc
					err := secretBackendsColl.FindId(doc.ValueRef.BackendID).One(&backendDoc)
					if err == nil {
						name = backendDoc.Name
					} else {
						name = "unknown"
					}
					backendNames[doc.ValueRef.BackendID] = name
				}
				backendName = &name
			}
		}
		result[i] = &secrets.SecretRevisionMetadata{
			Revision:    doc.Revision,
			ValueRef:    valueRef,
			BackendName: backendName,
			CreateTime:  doc.CreateTime,
			UpdateTime:  doc.UpdateTime,
			ExpireTime:  doc.ExpireTime,
		}
	}
	return result, nil
}

// allSecretRevisions is used for model export.
func (s *secretsStore) allSecretRevisions() ([]secretRevisionDoc, error) {
	secretRevisionCollection, closer := s.st.db().GetCollection(secretRevisionsC)
	defer closer()

	var docs []secretRevisionDoc

	err := secretRevisionCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

type secretConsumerDoc struct {
	DocID string `bson:"_id"`

	ConsumerTag     string `bson:"consumer-tag"`
	Label           string `bson:"label"`
	CurrentRevision int    `bson:"current-revision"`

	// LatestRevision is denormalised here so that the
	// consumer watcher can be triggered when a new
	// secret revision is added.
	LatestRevision int `bson:"latest-revision"`
}

func (st *State) secretConsumerKey(uri *secrets.URI, consumer string) string {
	if uri.IsLocal(st.ModelUUID()) {
		return fmt.Sprintf("%s#%s", uri.ID, consumer)
	}
	return fmt.Sprintf("%s/%s#%s", uri.SourceUUID, uri.ID, consumer)
}

func splitSecretConsumerKey(key string) (string, string) {
	parts := strings.Split(key, "#")
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// checkConsumerCountOps returns txn ops to ensure that no new secrets consumers
// are added whilst a txn is in progress.
func (st *State) checkConsumerCountOps(uri *secrets.URI, inc int) ([]txn.Op, error) {
	refCountCollection, ccloser := st.db().GetCollection(refcountsC)
	defer ccloser()

	key := fmt.Sprintf("%s#consumer", uri.ID)
	countOp, _, err := nsRefcounts.CurrentOp(refCountCollection, key)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// If not incrementing the consumer count, just ensure the count is stable.
	if inc == 0 {
		return []txn.Op{countOp}, nil
	}
	if inc > 0 {
		incOp, err := nsRefcounts.CreateOrIncRefOp(refCountCollection, key, inc)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return []txn.Op{countOp, incOp}, nil
	}
	incOp := nsRefcounts.JustIncRefOp(refcountsC, key, inc)
	return []txn.Op{countOp, incOp}, nil
}

const (
	secretOwnerLabelKeyPrefix    = "secretOwnerlabel"
	secretConsumerLabelKeyPrefix = "secretConsumerlabel"
)

func secretOwnerLabelKey(ownerTag names.Tag, label string) string {
	return fmt.Sprintf("%s#%s#%s", secretOwnerLabelKeyPrefix, ownerTag.String(), label)
}

func secretConsumerLabelKey(consumerTag names.Tag, label string) string {
	return fmt.Sprintf("%s#%s#%s", secretConsumerLabelKeyPrefix, consumerTag.String(), label)
}

func (st *State) uniqueSecretOwnerLabelOps(ownerTag names.Tag, label string) (ops []txn.Op, err error) {
	if ops, err = st.uniqueSecretLabelBaseOps(ownerTag, label); err != nil {
		return nil, errors.Trace(err)
	}

	// Check that there is no consumer with the same label.
	assertNoConsumerLabel, err := st.uniqueSecretLabelOpsRaw(ownerTag, label, "consumer", secretConsumerLabelKey, true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, assertNoConsumerLabel...)
	// Check that there is no owner with the same label.
	ops2, err := st.uniqueSecretLabelOpsRaw(ownerTag, label, "owner", secretOwnerLabelKey, false)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return append(ops, ops2...), nil
}

func (st *State) uniqueSecretConsumerLabelOps(consumerTag names.Tag, label string) (ops []txn.Op, err error) {
	if ops, err = st.uniqueSecretLabelBaseOps(consumerTag, label); err != nil {
		return nil, errors.Trace(err)
	}

	// Check that there is no owner with the same label.
	assertNoOwnerLabel, err := st.uniqueSecretLabelOpsRaw(consumerTag, label, "owner", secretOwnerLabelKey, true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, assertNoOwnerLabel...)
	// Check that there is no consumer with the same label.
	ops2, err := st.uniqueSecretLabelOpsRaw(consumerTag, label, "consumer", secretConsumerLabelKey, false)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return append(ops, ops2...), nil
}

func (st *State) uniqueSecretLabelBaseOps(tag names.Tag, label string) (ops []txn.Op, _ error) {
	col, close := st.db().GetCollection(refcountsC)
	defer close()

	var keyPattern string
	switch tag := tag.(type) {
	case names.ApplicationTag:
		// Ensure no units use this label for both owner and consumer label..
		keyPattern = fmt.Sprintf(
			"^%s:(%s|%s)#unit-%s-[0-9]+#%s$",
			st.ModelUUID(), secretOwnerLabelKeyPrefix, secretConsumerLabelKeyPrefix, tag.Name, label,
		)
	case names.UnitTag:
		// Ensure no application owned secret uses this label.
		applicationName, _ := names.UnitApplication(tag.Id())
		appTag := names.NewApplicationTag(applicationName)

		keyPattern = fmt.Sprintf(
			"^%s:(%s|%s)#%s#%s$",
			st.ModelUUID(), secretOwnerLabelKeyPrefix, secretConsumerLabelKeyPrefix, appTag.String(), label,
		)
	case names.ModelTag:
		keyPattern = fmt.Sprintf("^%s:%s#%s#%s$", st.ModelUUID(), secretOwnerLabelKeyPrefix, tag.String(), label)
	default:
		return nil, errors.NotSupportedf("tag type %T", tag)
	}

	count, err := col.Find(bson.M{"_id": bson.M{"$regex": keyPattern}}).Count()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if count > 0 {
		return nil, errors.WithType(errors.Errorf("secret label %q for %q already exists", label, tag), LabelExists)
	}

	return []txn.Op{
		{
			C:      col.Name(),
			Id:     bson.M{"$regex": keyPattern},
			Assert: txn.DocMissing,
		},
	}, nil
}

func (st *State) uniqueSecretLabelOpsRaw(tag names.Tag, label, role string, keyGenerator func(names.Tag, string) string, assertionOnly bool) ([]txn.Op, error) {
	refCountCollection, ccloser := st.db().GetCollection(refcountsC)
	defer ccloser()

	key := keyGenerator(tag, label)
	countOp, count, err := nsRefcounts.CurrentOp(refCountCollection, key)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if count > 0 {
		return nil, errors.WithType(errors.Errorf("secret label %q for %s %q already exists", label, role, tag), LabelExists)
	}
	if assertionOnly {
		// We only assert the doc doesn't exist but donot create the doc.
		return []txn.Op{countOp}, nil
	}
	incOp, err := nsRefcounts.CreateOrIncRefOp(refCountCollection, key, 1)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return []txn.Op{countOp, incOp}, nil
}

func (st *State) removeOwnerSecretLabelOps(ownerTag names.Tag) ([]txn.Op, error) {
	return st.removeSecretLabelOps(ownerTag, secretOwnerLabelKey)
}

func (st *State) removeConsumerSecretLabelOps(consumerTag names.Tag) ([]txn.Op, error) {
	return st.removeSecretLabelOps(consumerTag, secretConsumerLabelKey)
}

func (st *State) removeSecretLabelOps(tag names.Tag, keyGenerator func(names.Tag, string) string) ([]txn.Op, error) {
	refCountsCollection, closer := st.db().GetCollection(refcountsC)
	defer closer()

	var (
		doc bson.M
		ops []txn.Op
	)
	id := keyGenerator(tag, ".*")
	q := bson.D{{"_id", bson.D{{"$regex", id}}}}
	iter := refCountsCollection.Find(q).Iter()
	for iter.Next(&doc) {
		id, ok := doc["_id"].(string)
		if !ok {
			continue
		}
		count, _ := doc["refcount"].(int)
		op := nsRefcounts.JustRemoveOp(refcountsC, id, count)
		ops = append(ops, op)
	}
	return ops, iter.Close()
}

// GetURIByConsumerLabel gets the secret URI for the specified secret consumer label.
func (st *State) GetURIByConsumerLabel(label string, consumer names.Tag) (*secrets.URI, error) {
	secretConsumersCollection, closer := st.db().GetCollection(secretConsumersC)
	defer closer()

	var doc secretConsumerDoc
	err := secretConsumersCollection.Find(bson.M{
		"consumer-tag": consumer.String(), "label": label,
	}).Select(bson.D{{"_id", 1}}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("secret consumer with label %q for %q", label, consumer)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	uriStr, _ := splitSecretConsumerKey(st.localID(doc.DocID))
	if uriStr == "" {
		return nil, errors.NotFoundf("secret consumer with label %q for %q", label, consumer)
	}
	return secrets.ParseURI(uriStr)
}

// GetSecretConsumer gets secret consumer metadata.
func (st *State) GetSecretConsumer(uri *secrets.URI, consumer names.Tag) (*secrets.SecretConsumerMetadata, error) {
	if uri == nil {
		return nil, errors.NewNotValid(nil, "empty URI")
	}

	if uri.IsLocal(st.ModelUUID()) {
		if err := st.checkExists(uri); err != nil {
			return nil, errors.Trace(err)
		}
	}

	secretConsumersCollection, closer := st.db().GetCollection(secretConsumersC)
	defer closer()
	key := st.secretConsumerKey(uri, consumer.String())
	var doc secretConsumerDoc
	err := secretConsumersCollection.FindId(key).One(&doc)
	if errors.Cause(err) == mgo.ErrNotFound {
		return nil, errors.NotFoundf("consumer %q metadata for secret %q", consumer, uri.String())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &secrets.SecretConsumerMetadata{
		Label:           doc.Label,
		CurrentRevision: doc.CurrentRevision,
		LatestRevision:  doc.LatestRevision,
	}, nil
}

type secretRemoteConsumerDoc struct {
	DocID string `bson:"_id"`

	ConsumerTag     string `bson:"consumer-tag"`
	CurrentRevision int    `bson:"current-revision"`

	// LatestRevision is denormalised here so that the
	// consumer watcher can be triggered when a new
	// secret revision is added.
	LatestRevision int `bson:"latest-revision"`
}

// GetSecretRemoteConsumer gets secret consumer metadata
// for a cross model consumer.
func (st *State) GetSecretRemoteConsumer(uri *secrets.URI, consumer names.Tag) (*secrets.SecretConsumerMetadata, error) {
	if uri == nil {
		return nil, errors.NewNotValid(nil, "empty URI")
	}

	if err := st.checkExists(uri); err != nil {
		return nil, errors.Trace(err)
	}

	secretConsumersCollection, closer := st.db().GetCollection(secretRemoteConsumersC)
	defer closer()

	key := st.secretConsumerKey(uri, consumer.String())
	var doc secretRemoteConsumerDoc
	err := secretConsumersCollection.FindId(key).One(&doc)
	if errors.Cause(err) == mgo.ErrNotFound {
		return nil, errors.NotFoundf("consumer %q metadata for secret %q", consumer, uri.String())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	md := &secrets.SecretConsumerMetadata{
		CurrentRevision: doc.CurrentRevision,
		LatestRevision:  doc.LatestRevision,
	}

	return md, nil
}

func (st *State) removeSecretConsumerInfo(uri *secrets.URI) error {
	secretConsumersCollection, closer := st.db().GetCollection(secretConsumersC)
	defer closer()

	var docs []secretConsumerDoc
	err := secretConsumersCollection.Find(
		bson.D{
			{
				Name: "$and", Value: []bson.D{
					{{"_id", bson.D{{"$regex", fmt.Sprintf("%s#.*", uri.ID)}}}},
					{{"label", bson.D{{"$exists", true}, {"$ne", ""}}}},
				},
			},
		},
	).Select(bson.D{{"consumer-tag", 1}, {"label", 1}}).All(&docs)
	if err != nil && errors.Cause(err) != mgo.ErrNotFound {
		return errors.Trace(err)
	}
	refCountsCollection, closer := st.db().GetCollection(refcountsC)
	defer closer()
	for _, doc := range docs {
		consumer, _ := names.ParseTag(doc.ConsumerTag)
		key := secretConsumerLabelKey(consumer, doc.Label)
		_, err = refCountsCollection.Writeable().RemoveAll(bson.D{{
			"_id", key,
		}})
		if err != nil {
			return errors.Annotatef(err, "cannot delete consumer label refcounts for %s", key)
		}
	}

	_, err = secretConsumersCollection.Writeable().RemoveAll(bson.D{{
		"_id", bson.D{{"$regex", fmt.Sprintf("%s#.*", uri.ID)}},
	}})
	if err != nil {
		return errors.Annotatef(err, "cannot delete consumer info for %s", uri.String())
	}
	return nil
}

func (st *State) removeSecretRemoteConsumerInfo(uri *secrets.URI) error {
	secretConsumersCollection, closer := st.db().GetCollection(secretRemoteConsumersC)
	defer closer()

	_, err := secretConsumersCollection.Writeable().RemoveAll(bson.D{{
		"_id", bson.D{{"$regex", fmt.Sprintf("%s#.*", uri.ID)}},
	}})
	if err != nil {
		return errors.Annotatef(err, "cannot delete remote consumer info for %s", uri.String())
	}
	return nil
}

// RemoveSecretConsumer removes secret references for the specified consumer.
func (st *State) RemoveSecretConsumer(consumer names.Tag) error {
	secretConsumersCollection, closer := st.db().GetCollection(secretConsumersC)
	defer closer()

	var docs []secretConsumerDoc
	err := secretConsumersCollection.Find(
		bson.D{{"consumer-tag", consumer.String()}},
	).Select(bson.D{{"_id", 1}, {"label", 1}}).All(&docs)
	if err != nil && errors.Cause(err) != mgo.ErrNotFound {
		return errors.Trace(err)
	}
	refCountsCollection, closer := st.db().GetCollection(refcountsC)
	defer closer()
	for _, doc := range docs {
		key := secretConsumerLabelKey(consumer, doc.Label)
		_, err = refCountsCollection.Writeable().RemoveAll(bson.D{{
			"_id", key,
		}})
		if err != nil {
			return errors.Annotatef(err, "cannot delete consumer label refcounts for %s", key)
		}
	}

	_, err = secretConsumersCollection.Writeable().RemoveAll(
		bson.D{{"consumer-tag", consumer.String()}})
	if err != nil {
		return errors.Annotatef(err, "cannot delete consumer info for %s", consumer.String())
	}
	return nil
}

// removeRemoteSecretConsumer removes secret consumer info for the specified
// remote application and also any of its units.
func (st *State) removeRemoteSecretConsumer(appName string) error {
	secretConsumersCollection, closer := st.db().GetCollection(secretRemoteConsumersC)
	defer closer()

	match := fmt.Sprintf("(unit|application)-%s(\\/\\d)?", appName)
	q := bson.D{{"consumer-tag", bson.D{{"$regex", match}}}}
	_, err := secretConsumersCollection.Writeable().RemoveAll(q)
	return err
}

// updateSecretConsumerOperation is used to update secret consumers
// in the consuming model when the secret in the offering model gets a new
// revision added.
type updateSecretConsumerOperation struct {
	st             *State
	uri            *secrets.URI
	latestRevision int
}

// Build implements ModelOperation.
func (u *updateSecretConsumerOperation) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		return nil, errors.NotFoundf("secret consumers for secret %q", u.uri)
	}
	return u.st.secretUpdateConsumersOps(secretConsumersC, u.uri, u.latestRevision)
}

// Done implements ModelOperation.
func (u *updateSecretConsumerOperation) Done(err error) error {
	return err
}

// UpdateSecretConsumerOperation returns a model operation to update
// secret consumer metadata when a secret in the offering model
// gets a new revision added..
func (st *State) UpdateSecretConsumerOperation(uri *secrets.URI, latestRevision int) (ModelOperation, error) {
	return &updateSecretConsumerOperation{
		st:             st,
		uri:            uri,
		latestRevision: latestRevision,
	}, nil
}

// SaveSecretConsumer saves or updates secret consumer metadata.
func (st *State) SaveSecretConsumer(uri *secrets.URI, consumer names.Tag, metadata *secrets.SecretConsumerMetadata) error {
	key := st.secretConsumerKey(uri, consumer.String())
	secretConsumersCollection, closer := st.db().GetCollection(secretConsumersC)
	defer closer()

	// Cross model secrets do not exist in this model.
	localSecret := uri.IsLocal(st.ModelUUID())

	var doc secretConsumerDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if localSecret {
			if err := st.checkExists(uri); err != nil {
				return nil, errors.Trace(err)
			}
		}
		err := secretConsumersCollection.FindId(key).One(&doc)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		create := err != nil

		var ops []txn.Op

		if metadata.Label != "" && (create || metadata.Label != doc.Label) {
			uniqueLabelOps, err := st.uniqueSecretConsumerLabelOps(consumer, metadata.Label)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, uniqueLabelOps...)
		}
		if create {
			ops = append(ops, txn.Op{
				C:      secretConsumersC,
				Id:     key,
				Assert: txn.DocMissing,
				Insert: secretConsumerDoc{
					DocID:           key,
					ConsumerTag:     consumer.String(),
					Label:           metadata.Label,
					CurrentRevision: metadata.CurrentRevision,
					LatestRevision:  metadata.LatestRevision,
				},
			})

			if localSecret {
				// Increment the consumer count, ensuring no new consumers
				// are added while update is in progress.
				countRefOps, err := st.checkConsumerCountOps(uri, 1)
				if err != nil {
					return nil, errors.Trace(err)
				}
				ops = append(ops, countRefOps...)
			}
		} else {
			ops = append(ops, txn.Op{
				C:      secretConsumersC,
				Id:     key,
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{
					"label":            metadata.Label,
					"current-revision": metadata.CurrentRevision,
				}},
			})
			if localSecret && metadata.CurrentRevision > doc.CurrentRevision {
				// The consumer is tracking a new revision, which might result in the
				// previous revision becoming obsolete.
				obsoleteOps, err := st.markObsoleteRevisionOps(uri, consumer.String(), metadata.CurrentRevision)
				if err != nil {
					return nil, errors.Trace(err)
				}
				ops = append(ops, obsoleteOps...)
			}
		}

		return ops, nil
	}
	return st.db().Run(buildTxn)
}

// SaveSecretRemoteConsumer saves or updates secret consumer metadata
// for a cross model consumer.
func (st *State) SaveSecretRemoteConsumer(uri *secrets.URI, consumer names.Tag, metadata *secrets.SecretConsumerMetadata) error {
	key := st.secretConsumerKey(uri, consumer.String())
	secretConsumersCollection, closer := st.db().GetCollection(secretRemoteConsumersC)
	defer closer()

	var doc secretRemoteConsumerDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if err := st.checkExists(uri); err != nil {
			return nil, errors.Trace(err)
		}
		err := secretConsumersCollection.FindId(key).One(&doc)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		create := err != nil

		var ops []txn.Op

		if create {
			ops = append(ops, txn.Op{
				C:      secretRemoteConsumersC,
				Id:     key,
				Assert: txn.DocMissing,
				Insert: secretRemoteConsumerDoc{
					DocID:           key,
					ConsumerTag:     consumer.String(),
					CurrentRevision: metadata.CurrentRevision,
					LatestRevision:  metadata.LatestRevision,
				},
			})

			// Increment the consumer count, ensuring no new consumers
			// are added while update is in progress.
			countRefOps, err := st.checkConsumerCountOps(uri, 1)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, countRefOps...)
		} else {
			ops = append(ops, txn.Op{
				C:      secretRemoteConsumersC,
				Id:     key,
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{
					"current-revision": metadata.CurrentRevision,
				}},
			})
			if metadata.CurrentRevision > doc.CurrentRevision {
				// The consumer is tracking a new revision, which might result in the
				// previous revision becoming obsolete.
				obsoleteOps, err := st.markObsoleteRevisionOps(uri, consumer.String(), metadata.CurrentRevision)
				if err != nil {
					return nil, errors.Trace(err)
				}
				ops = append(ops, obsoleteOps...)
			}
		}

		return ops, nil
	}
	return st.db().Run(buildTxn)
}

// secretUpdateConsumersOps updates the latest secret revision number
// on all consumers. This triggers the secrets change watcher.
func (st *State) secretUpdateConsumersOps(coll string, uri *secrets.URI, newRevision int) ([]txn.Op, error) {
	secretConsumersCollection, closer := st.db().GetCollection(coll)
	defer closer()

	var (
		doc secretConsumerDoc
		ops []txn.Op
	)
	key := st.secretConsumerKey(uri, ".*")
	q := bson.D{{"_id", bson.D{{"$regex", key}}}}
	iter := secretConsumersCollection.Find(q).Iter()
	for iter.Next(&doc) {
		ops = append(ops, txn.Op{
			C:      coll,
			Id:     doc.DocID,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"latest-revision": newRevision}},
		})
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Annotate(err, "getting secret consumers")
	}
	return ops, nil
}

const (
	idSnippet   = `[0-9a-z]{20}`
	uuidSnippet = `[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`
)

// allLocalSecretConsumers is used for model export.
func (s *secretsStore) allLocalSecretConsumers() ([]secretConsumerDoc, error) {
	secretConsumerCollection, closer := s.st.db().GetCollection(secretConsumersC)
	defer closer()

	var docs []secretConsumerDoc

	err := secretConsumerCollection.Find(bson.D{{"_id", bson.D{{"$regex", idSnippet}}}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

// allRemoteSecretConsumers is used for model export.
func (s *secretsStore) allRemoteSecretConsumers() ([]secretConsumerDoc, error) {
	secretConsumerCollection, closer := s.st.db().GetCollection(secretConsumersC)
	defer closer()

	var docs []secretConsumerDoc

	q := fmt.Sprintf(`%s/%s`, uuidSnippet, idSnippet)
	err := secretConsumerCollection.Find(bson.D{{"_id", bson.D{{"$regex", q}}}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

// allSecretRemoteConsumers is used for model export.
func (s *secretsStore) allSecretRemoteConsumers() ([]secretRemoteConsumerDoc, error) {
	secretRemoteConsumerCollection, closer := s.st.db().GetCollection(secretRemoteConsumersC)
	defer closer()

	var docs []secretRemoteConsumerDoc

	err := secretRemoteConsumerCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

// WatchConsumedSecretsChanges returns a watcher for updates and deletes
// of secrets that have been previously read by the specified consumer.
func (st *State) WatchConsumedSecretsChanges(consumer names.Tag) (StringsWatcher, error) {
	return newConsumedSecretsWatcher(st, consumer.String(), false), nil
}

// WatchRemoteConsumedSecretsChanges returns a watcher for updates and deletes
// of secrets that have been previously read by the specified remote consumer app.
func (st *State) WatchRemoteConsumedSecretsChanges(consumerApp string) (StringsWatcher, error) {
	return newConsumedSecretsWatcher(st, consumerApp, true), nil
}

type consumedSecretsWatcher struct {
	commonWatcher
	out chan []string

	knownRevisions map[string]int

	coll       string
	matchQuery bson.D
	filter     func(id string) bool
}

func newConsumedSecretsWatcher(st modelBackend, consumer string, remote bool) StringsWatcher {
	w := &consumedSecretsWatcher{
		commonWatcher:  newCommonWatcher(st),
		out:            make(chan []string),
		knownRevisions: make(map[string]int),
	}
	if !remote {
		w.coll = secretConsumersC
		w.matchQuery = bson.D{{"consumer-tag", consumer}}
		w.filter = func(id string) bool {
			return strings.HasSuffix(id, "#"+consumer)
		}
	} else {
		w.coll = secretRemoteConsumersC
		match := fmt.Sprintf("(unit|application)-%s(\\/\\d)?", consumer)
		w.matchQuery = bson.D{{"consumer-tag", bson.D{{"$regex", match}}}}
		w.filter = func(id string) bool {
			return strings.HasSuffix(id, "#application-"+consumer) || strings.Contains(id, "#unit-"+consumer+"-")
		}
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

// Changes implements StringsWatcher.
func (w *consumedSecretsWatcher) Changes() <-chan []string {
	return w.out
}

func (w *consumedSecretsWatcher) initial() ([]string, error) {
	var doc secretConsumerDoc
	secretConsumersCollection, closer := w.db.GetCollection(w.coll)
	defer closer()

	var ids []string
	iter := secretConsumersCollection.Find(w.matchQuery).Select(bson.D{{"latest-revision", 1}}).Iter()
	for iter.Next(&doc) {
		w.knownRevisions[doc.DocID] = doc.LatestRevision
		if doc.LatestRevision < 2 {
			continue
		}
		uriStr := strings.Split(w.backend.localID(doc.DocID), "#")[0]
		uri, err := secrets.ParseURI(uriStr)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ids = append(ids, uri.String())
	}
	return ids, errors.Trace(iter.Close())
}

func (w *consumedSecretsWatcher) merge(currentChanges []string, change watcher.Change) ([]string, error) {
	changeID := change.Id.(string)
	uriStr := strings.Split(w.backend.localID(changeID), "#")[0]
	seenRevision, known := w.knownRevisions[changeID]

	if change.Revno < 0 {
		// Record deleted.
		if !known {
			return currentChanges, nil
		}
		delete(w.knownRevisions, changeID)
		uri, err := secrets.ParseURI(uriStr)
		if err != nil {
			return nil, errors.Trace(err)
		}
		currentChanges = append(currentChanges, uri.String())
		return currentChanges, nil
	}

	// Record added or updated.
	var doc secretConsumerDoc
	secretConsumerColl, closer := w.db.GetCollection(w.coll)
	defer closer()

	err := secretConsumerColl.FindId(change.Id).Select(bson.D{{"latest-revision", 1}}).One(&doc)
	if err != nil && err != mgo.ErrNotFound {
		return nil, errors.Trace(err)
	}
	// Changed but no longer in the model so ignore.
	if err != nil {
		return currentChanges, nil
	}
	w.knownRevisions[changeID] = doc.LatestRevision
	if doc.LatestRevision > 1 && doc.LatestRevision != seenRevision {
		uri, err := secrets.ParseURI(uriStr)
		if err != nil {
			return nil, errors.Trace(err)
		}
		currentChanges = append(currentChanges, uri.String())
	}
	return currentChanges, nil
}

func (w *consumedSecretsWatcher) loop() (err error) {
	ch := make(chan watcher.Change)
	filter := func(id interface{}) bool {
		k, err := w.backend.strictLocalID(id.(string))
		if err != nil {
			return false
		}
		return w.filter(k)
	}
	w.watcher.WatchCollectionWithFilter(w.coll, ch, filter)
	defer w.watcher.UnwatchCollection(w.coll, ch)

	changes, err := w.initial()
	if err != nil {
		return errors.Trace(err)
	}

	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case change, ok := <-ch:
			if !ok {
				return tomb.ErrDying
			}
			if changes, err = w.merge(changes, change); err != nil {
				return err
			}
			if len(changes) > 0 {
				out = w.out
			}
		case out <- changes:
			out = nil
			changes = nil
		}
	}
}

// WatchObsolete returns a watcher for notifying when:
//   - a secret owned by the entity is deleted
//   - a secret revision owed by the entity no longer
//     has any consumers
//
// Obsolete revisions results are "uri/revno" and deleted
// secret results are "uri".
func (s *secretsStore) WatchObsolete(ownerTags []names.Tag) (StringsWatcher, error) {
	if len(ownerTags) == 0 {
		return nil, errors.New("missing secret owners")
	}
	owners := make([]string, len(ownerTags))
	for i, owner := range ownerTags {
		owners[i] = owner.String()
	}
	return newObsoleteSecretsWatcher(s.st, owners, nil), nil
}

// WatchRevisionsToPrune returns a watcher for notifying when a user supplied secret revision needs to be pruned.
func (s *secretsStore) WatchRevisionsToPrune(ownerTags []names.Tag) (StringsWatcher, error) {
	if len(ownerTags) == 0 {
		return nil, errors.New("missing secret owners")
	}
	owners := make([]string, len(ownerTags))
	for i, owner := range ownerTags {
		owners[i] = owner.String()
	}
	fitler := func(id string) bool {
		uri, err := secrets.ParseURI(id)
		if err != nil {
			logger.Warningf("invalid secret URI %q, err %#v", id, err)
			return false
		}
		md, err := s.GetSecret(uri)
		if err != nil {
			logger.Warningf("cannot get secret %q, err %#v", uri, err)
			return false
		}
		if s.st.modelTag.String() != md.OwnerTag {
			// Only prune secrets owned by this model (user secrets).
			return false
		}
		return md.AutoPrune
	}
	return newObsoleteSecretsWatcher(s.st, owners, fitler), nil
}

type obsoleteSecretsWatcher struct {
	commonWatcher
	out chan []string

	obsoleteRevisionsWatcher *collectionWatcher

	owners []string
	known  set.Strings
}

func newObsoleteSecretsWatcher(st modelBackend, owners []string, filter func(string) bool) *obsoleteSecretsWatcher {
	// obsoleteRevisionsWatcher is for tracking secret revisions with no consumers.
	obsoleteRevisionsWatcher := newCollectionWatcher(st, colWCfg{
		col: secretRevisionsC,
		filter: func(key interface{}) bool {
			secretRevisionsCollection, closer := st.db().GetCollection(secretRevisionsC)
			defer closer()

			var doc secretRevisionDoc
			err := secretRevisionsCollection.Find(bson.D{{"_id", key}, secretOwnerTerm(owners)}).Select(
				bson.D{{"obsolete", 1}},
			).One(&doc)
			if err != nil {
				return false
			}
			if filter == nil {
				return doc.Obsolete
			}
			uri, _ := splitSecretRevision(st.localID(doc.DocID))
			return doc.Obsolete && filter(uri)
		},
		idconv: func(idStr string) string {
			id, rev := splitSecretRevision(idStr)
			if rev == 0 {
				return idStr
			}
			uri := secrets.URI{ID: id}
			return uri.String() + fmt.Sprintf("/%d", rev)
		},
	})

	w := &obsoleteSecretsWatcher{
		commonWatcher:            newCommonWatcher(st),
		obsoleteRevisionsWatcher: obsoleteRevisionsWatcher.(*collectionWatcher),
		out:                      make(chan []string),
		known:                    set.NewStrings(),
		owners:                   owners,
	}
	w.tomb.Go(func() error {
		defer w.finish()
		return w.loop()
	})
	return w
}

func (w *obsoleteSecretsWatcher) finish() {
	watcher.Stop(w.obsoleteRevisionsWatcher, &w.tomb)
	close(w.out)
}

// Changes implements StringsWatcher.
func (w *obsoleteSecretsWatcher) Changes() <-chan []string {
	return w.out
}

func (w *obsoleteSecretsWatcher) initial() error {
	var doc secretMetadataDoc
	secretMetadataCollection, closer := w.db.GetCollection(secretMetadataC)
	defer closer()

	iter := secretMetadataCollection.Find(bson.D{secretOwnerTerm(w.owners)}).Iter()
	for iter.Next(&doc) {
		w.known.Add(doc.DocID)
	}
	return errors.Trace(iter.Close())
}

func (w *obsoleteSecretsWatcher) mergedOwnedChanges(currentChanges []string, change watcher.Change) ([]string, error) {
	changeID := change.Id.(string)
	known := w.known.Contains(changeID)

	if change.Revno < 0 {
		if !known {
			return currentChanges, nil
		}
		// Secret deleted.
		delete(w.known, changeID)
		uriStr := w.backend.localID(changeID)
		uri, err := secrets.ParseURI(uriStr)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// If there are any pending changes for obsolete revisions and the
		// secret becomes deleted, the obsolete revision changes are no
		// longer relevant.
		var newChanges []string
		for _, c := range currentChanges {
			id, _ := splitSecretRevision(c)
			if id != uri.String() {
				newChanges = append(newChanges, c)
			}
		}
		newChanges = append(newChanges, uri.String())
		return newChanges, nil
	}
	if known {
		return currentChanges, nil
	}
	var doc secretMetadataDoc
	// Record added or updated - we don't emit an event but
	// record that we know about it.
	secretMetadataColl, closer := w.db.GetCollection(secretMetadataC)
	defer closer()
	err := secretMetadataColl.Find(bson.D{{"_id", change.Id}, secretOwnerTerm(w.owners)}).One(&doc)
	if err != nil && err != mgo.ErrNotFound {
		return nil, errors.Trace(err)
	}
	// Changed but no longer in the model so ignore.
	if err != nil {
		return currentChanges, nil
	}
	w.known.Add(changeID)
	return currentChanges, nil
}

func (w *obsoleteSecretsWatcher) mergeRevisionChanges(currentChanges []string, obsolete []string) []string {
	newChanges := set.NewStrings(currentChanges...).Union(set.NewStrings(obsolete...))
	return newChanges.Values()
}

func (w *obsoleteSecretsWatcher) loop() (err error) {
	// Watch changes to secrets owned by the entity.
	ownedChanges := make(chan watcher.Change)
	w.watcher.WatchCollection(secretMetadataC, ownedChanges)
	defer w.watcher.UnwatchCollection(secretMetadataC, ownedChanges)

	if err = w.initial(); err != nil {
		return errors.Trace(err)
	}

	var (
		changes                  []string
		gotInitialObsoleteChange bool
	)
	out := w.out
	gotInitialObsoleteChange = false
	for {
		// Give any incoming secret deletion events
		// time to arrive so the revision obsolete and
		// delete events can be squashed.
		timeout := time.After(time.Second)
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case obsoleteRevisions, ok := <-w.obsoleteRevisionsWatcher.Changes():
			if !ok {
				return tomb.ErrDying
			}
			if !gotInitialObsoleteChange {
				gotInitialObsoleteChange = true
				break
			}
			changes = w.mergeRevisionChanges(changes, obsoleteRevisions)
		case change, ok := <-ownedChanges:
			if !ok {
				return tomb.ErrDying
			}
			if changes, err = w.mergedOwnedChanges(changes, change); err != nil {
				return err
			}
		case <-timeout:
			if len(changes) > 0 {
				out = w.out
			}
		case out <- changes:
			out = nil
			changes = nil
		}
	}
}

// markObsoleteRevisionOps returns ops for marking any revisions which are currently
// not being tracked by any consumers as obsolete.
func (st *State) markObsoleteRevisionOps(uri *secrets.URI, exceptForConsumer string, exceptForRev int) ([]txn.Op, error) {
	var obsoleteOps []txn.Op
	revs, latest, err := st.getOrphanedSecretRevisions(uri, exceptForConsumer, exceptForRev)
	if err != nil {
		return nil, errors.Annotate(err, "getting orphaned secret revisions")
	}

	for _, rev := range revs {
		obsoleteOps = append(obsoleteOps, txn.Op{
			C:      secretRevisionsC,
			Id:     secretRevisionKey(uri, rev),
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{
				"obsolete": true,
			}},
		})
	}
	// Ensure that no concurrent revision updates can happen.
	if len(obsoleteOps) > 0 {
		obsoleteOps = append(obsoleteOps, txn.Op{
			C:      secretMetadataC,
			Id:     uri.ID,
			Assert: bson.D{{"latest-revision", latest}},
		})
	}
	return obsoleteOps, nil
}

// getOrphanedSecretRevisions returns revisions which are not being tracked by any consumer,
// plus the current latest revision, for the specified secret, excluding the specified
// consumer and/or revision.
func (st *State) getOrphanedSecretRevisions(uri *secrets.URI, exceptForConsumer string, exceptForRev int) ([]int, int, error) {
	store := NewSecrets(st)
	revInfo, err := store.listSecretRevisions(uri, nil)
	if err != nil {
		return nil, 0, errors.Trace(err)
	}
	allRevisions := set.NewInts()
	for _, r := range revInfo {
		allRevisions.Add(r.Revision)
	}
	latest := allRevisions.SortedValues()[allRevisions.Size()-1]
	allRevisions.Remove(exceptForRev)

	consumedRevs, err := st.getInUseSecretRevisions(secretConsumersC, uri, exceptForConsumer)
	if err != nil {
		return nil, 0, errors.Trace(err)
	}
	remoteConsumedRevs, err := st.getInUseSecretRevisions(secretRemoteConsumersC, uri, exceptForConsumer)
	if err != nil {
		return nil, 0, errors.Trace(err)
	}
	usedRevisions := consumedRevs.Union(remoteConsumedRevs)
	return allRevisions.Difference(usedRevisions).Values(), latest, nil
}

func (st *State) getInUseSecretRevisions(collName string, uri *secrets.URI, exceptForConsumer string) (set.Ints, error) {
	secretConsumersCollection, closer := st.db().GetCollection(collName)
	defer closer()

	pipe := secretConsumersCollection.Pipe([]bson.M{
		{
			"$match": bson.M{
				"_id":          bson.M{"$regex": st.docID(uri.ID + "#.*")},
				"consumer-tag": bson.M{"$ne": exceptForConsumer},
			},
		},
		{
			"$group": bson.M{
				"_id": bson.M{"$toString": "$current-revision"}, "count": bson.M{"$sum": 1},
			},
		},
		{
			"$sort": bson.M{"_id": 1},
		},
		{
			"$group": bson.M{
				"_id": nil,
				"counts": bson.M{
					"$push": bson.M{"k": "$_id", "v": "$count"},
				},
			},
		},
		{
			"$replaceRoot": bson.M{
				"newRoot": bson.M{"$arrayToObject": "$counts"},
			},
		},
	})
	var usedRevisions []map[string]int
	err := pipe.All(&usedRevisions)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := set.NewInts()
	if len(usedRevisions) == 0 {
		return result, nil
	}

	for revStr := range usedRevisions[0] {
		r, _ := strconv.Atoi(revStr)
		result.Add(r)
	}
	return result, nil
}

type secretPermissionDoc struct {
	DocID string `bson:"_id"`

	Scope   string `bson:"scope-tag"`
	Subject string `bson:"subject-tag"`
	Role    string `bson:"role"`
}

func (st *State) findSecretEntity(tag names.Tag) (entity Lifer, collName, docID string, err error) {
	id := tag.Id()
	switch tag.(type) {
	case names.RelationTag:
		entity, err = st.KeyRelation(id)
		collName = relationsC
		docID = id
	case names.UnitTag:
		entity, err = st.Unit(id)
		collName = unitsC
		docID = id
	case names.ApplicationTag:
		entity, err = st.Application(id)
		docID = id
		if err == nil {
			collName = applicationsC
		} else if errors.Is(err, errors.NotFound) {
			entity, err = st.RemoteApplication(id)
			collName = remoteApplicationsC
		}
	case names.ModelTag:
		if st.ModelUUID() != tag.Id() {
			// This should never happen, but just in case.
			return nil, "", "", errors.NotFoundf("model %q", tag.Id())
		}
		entity, err = st.Model()
		if err != nil {
			return nil, "", "", errors.Trace(err)
		}
		collName = modelsC
		docID = id
	default:
		err = errors.NotValidf("secret scope reference %q", tag.String())
	}
	return entity, collName, docID, err
}

func (st *State) referencedSecrets(ref names.Tag, attr string) ([]*secrets.URI, error) {
	secretMetadataCollection, closer := st.db().GetCollection(secretMetadataC)
	defer closer()

	var (
		doc    secretMetadataDoc
		result []*secrets.URI
	)
	iter := secretMetadataCollection.Find(bson.D{{attr, ref.String()}}).Select(bson.D{{"_id", 1}}).Iter()
	for iter.Next(&doc) {
		uri, err := secrets.ParseURI(st.localID(doc.DocID))
		if err != nil {
			_ = iter.Close()
			return nil, errors.Trace(err)
		}
		result = append(result, uri)
	}
	return result, iter.Close()

}

// SecretAccessParams are used to grant/revoke secret access.
type SecretAccessParams struct {
	LeaderToken leadership.Token
	Scope       names.Tag
	Subject     names.Tag
	Role        secrets.SecretRole
}

// GrantSecretAccess saves the secret access role for the subject with the specified scope.
func (st *State) GrantSecretAccess(uri *secrets.URI, p SecretAccessParams) (err error) {
	if p.Role == secrets.RoleNone || !p.Role.IsValid() {
		return errors.NotValidf("secret role %q", p.Role)
	}

	scopeEntity, scopeCollName, scopeDocID, err := st.findSecretEntity(p.Scope)
	if err != nil {
		return errors.Annotate(err, "invalid scope reference")
	}
	if scopeEntity.Life() != Alive {
		return errors.Errorf("cannot grant access to secret in scope of %q which is not alive", p.Scope)
	}
	subjectEntity, subjectCollName, subjectDocID, err := st.findSecretEntity(p.Subject)
	if p.Subject.Kind() == names.UnitTagKind && errors.Is(err, errors.NotFound) {
		unitApp, _ := names.UnitApplication(p.Subject.Id())
		_, err2 := st.RemoteApplication(unitApp)
		if err2 != nil && !errors.Is(err2, errors.NotFound) {
			return errors.Trace(err2)
		}
		if err2 == nil {
			return errors.NotSupportedf("sharing secrets with a unit across a cross model relation")
		}
	}
	if err != nil {
		return errors.Annotate(err, "invalid subject reference")
	}
	if subjectEntity.Life() != Alive {
		return errors.Errorf("cannot grant dying %q access to secret", p.Subject)
	}

	// Apps on the offering side of a cross model relation can grant secret access.
	type remoteApp interface {
		IsConsumerProxy() bool
	}
	var subjectApp remoteApp

	if subjectCollName == remoteApplicationsC {
		if e, ok := subjectEntity.(remoteApp); ok {
			subjectApp = e
		}
		if subjectApp == nil || !subjectApp.IsConsumerProxy() {
			return errors.NotSupportedf("sharing consumer secrets across a cross model relation")
		}
	}

	isScopeAliveOp := txn.Op{
		C:      scopeCollName,
		Id:     scopeDocID,
		Assert: isAliveDoc,
	}
	isSubjectAliveOp := txn.Op{
		C:      subjectCollName,
		Id:     subjectDocID,
		Assert: isAliveDoc,
	}

	key := st.secretConsumerKey(uri, p.Subject.String())

	secretPermissionsCollection, closer := st.db().GetCollection(secretPermissionsC)
	defer closer()

	var doc secretPermissionDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if err := st.checkExists(uri); err != nil {
			return nil, errors.Trace(err)
		}
		err = secretPermissionsCollection.FindId(key).One(&doc)
		if err == mgo.ErrNotFound {
			return []txn.Op{{
				C:      secretPermissionsC,
				Id:     key,
				Assert: txn.DocMissing,
				Insert: secretPermissionDoc{
					DocID:   key,
					Subject: p.Subject.String(),
					Scope:   p.Scope.String(),
					Role:    string(p.Role),
				},
			}}, nil
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		if doc.Scope != p.Scope.String() {
			return nil, errors.New("cannot change secret permission scope")
		}
		if doc.Subject != p.Subject.String() {
			return nil, errors.New("cannot change secret permission subject")
		}
		return []txn.Op{{
			C:      secretPermissionsC,
			Id:     key,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{
				"role": p.Role,
			}},
		}, isScopeAliveOp, isSubjectAliveOp}, nil
	}
	return st.db().Run(buildTxnWithLeadership(buildTxn, p.LeaderToken))
}

// RevokeSecretAccess removes any secret access role for the subject.
func (st *State) RevokeSecretAccess(uri *secrets.URI, p SecretAccessParams) error {
	key := st.secretConsumerKey(uri, p.Subject.String())

	secretPermissionsCollection, closer := st.db().GetCollection(secretPermissionsC)
	defer closer()

	var doc secretPermissionDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if err := st.checkExists(uri); err != nil {
			if errors.Is(err, errors.NotFound) {
				return nil, jujutxn.ErrNoOperations
			}
			return nil, errors.Trace(err)
		}
		err := secretPermissionsCollection.FindId(key).One(&doc)
		if err == mgo.ErrNotFound {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}

		ops := []txn.Op{{
			C:      secretPermissionsC,
			Id:     key,
			Assert: txn.DocExists,
			Remove: true,
		}}
		return ops, nil
	}
	return st.db().Run(buildTxnWithLeadership(buildTxn, p.LeaderToken))
}

// SecretAccess returns the secret access role for the subject.
func (st *State) SecretAccess(uri *secrets.URI, subject names.Tag) (secrets.SecretRole, error) {
	key := st.secretConsumerKey(uri, subject.String())

	secretPermissionsCollection, closer := st.db().GetCollection(secretPermissionsC)
	defer closer()

	if err := st.checkExists(uri); err != nil {
		return secrets.RoleNone, errors.Trace(err)
	}

	var doc secretPermissionDoc
	err := secretPermissionsCollection.FindId(key).One(&doc)
	if err == mgo.ErrNotFound {
		return secrets.RoleNone, nil
	}
	if err != nil {
		return secrets.RoleNone, errors.Trace(err)
	}
	return secrets.SecretRole(doc.Role), nil
}

// SecretAccessScope returns the secret access scope for the subject.
func (st *State) SecretAccessScope(uri *secrets.URI, subject names.Tag) (names.Tag, error) {
	key := st.secretConsumerKey(uri, subject.String())

	secretPermissionsCollection, closer := st.db().GetCollection(secretPermissionsC)
	defer closer()

	if err := st.checkExists(uri); err != nil {
		return nil, errors.Trace(err)
	}

	var doc secretPermissionDoc
	err := secretPermissionsCollection.FindId(key).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("secret access for consumer %q", subject)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return names.ParseTag(doc.Scope)
}

func (st *State) removeScopedSecretPermissionOps(scope names.Tag) ([]txn.Op, error) {
	secretPermissionsCollection, closer := st.db().GetCollection(secretPermissionsC)
	defer closer()

	var (
		doc secretPermissionDoc
		ops []txn.Op
	)
	iter := secretPermissionsCollection.Find(bson.D{{"scope-tag", scope.String()}}).Select(bson.D{{"_id", 1}}).Iter()
	for iter.Next(&doc) {
		ops = append(ops, txn.Op{
			C:      secretPermissionsC,
			Id:     doc.DocID,
			Remove: true,
		})
	}
	return ops, iter.Close()
}

func (st *State) removeConsumerSecretPermissionOps(consumer names.Tag) ([]txn.Op, error) {
	secretPermissionsCollection, closer := st.db().GetCollection(secretPermissionsC)
	defer closer()

	var (
		doc secretPermissionDoc
		ops []txn.Op
	)
	iter := secretPermissionsCollection.Find(bson.D{{"subject-tag", consumer.String()}}).Select(bson.D{{"_id", 1}}).Iter()
	for iter.Next(&doc) {
		ops = append(ops, txn.Op{
			C:      secretPermissionsC,
			Id:     doc.DocID,
			Remove: true,
		})
	}
	return ops, iter.Close()
}

type secretRotationDoc struct {
	DocID    string `bson:"_id"`
	TxnRevno int64  `bson:"txn-revno"`

	// These fields are denormalised here so that the watcher
	// only needs to access this collection.
	NextRotateTime time.Time `bson:"next-rotate-time"`
	OwnerTag       string    `bson:"owner-tag"`
}

func (s *secretsStore) secretRotationOps(uri *secrets.URI, owner string, rotatePolicy *secrets.RotatePolicy, nextRotateTime *time.Time) ([]txn.Op, error) {
	secretKey := uri.ID
	if rotatePolicy != nil && !rotatePolicy.WillRotate() {
		return []txn.Op{{
			C:      secretRotateC,
			Id:     secretKey,
			Remove: true,
		}}, nil
	}
	if nextRotateTime == nil {
		return nil, errors.New("must specify a secret rotate time")
	}
	secretRotateCollection, closer := s.st.db().GetCollection(secretRotateC)
	defer closer()

	var doc secretRotationDoc
	err := secretRotateCollection.FindId(secretKey).One(&doc)
	if err == mgo.ErrNotFound {
		return []txn.Op{{
			C:      secretRotateC,
			Id:     secretKey,
			Assert: txn.DocMissing,
			Insert: secretRotationDoc{
				DocID:          secretKey,
				NextRotateTime: (*nextRotateTime).Round(time.Second).UTC(),
				OwnerTag:       owner,
			},
		}}, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return []txn.Op{{
		C:      secretRotateC,
		Id:     secretKey,
		Assert: txn.DocExists,
		Update: bson.M{"$set": bson.M{"next-rotate-time": nextRotateTime.Round(time.Second).UTC()}},
	}}, nil
}

// SecretRotated records when the given secret was rotated.
func (st *State) SecretRotated(uri *secrets.URI, next time.Time) error {
	secretRotateCollection, closer := st.db().GetCollection(secretRotateC)
	defer closer()

	secretKey := uri.ID
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if err := st.checkExists(uri); err != nil {
			return nil, errors.Trace(err)
		}

		var currentDoc secretRotationDoc
		err := secretRotateCollection.FindId(secretKey).One(&currentDoc)
		if errors.Cause(err) == mgo.ErrNotFound {
			return nil, errors.NotFoundf("rotation info for secret %q", uri.String())
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		// If next rotate time is sooner than our proposed time, keep the existing value.
		if attempt > 0 && currentDoc.NextRotateTime.Before(next) {
			return nil, jujutxn.ErrNoOperations
		}
		ops := []txn.Op{{
			C:      secretRotateC,
			Id:     secretKey,
			Assert: bson.D{{"txn-revno", currentDoc.TxnRevno}},
			Update: bson.M{"$set": bson.M{
				"next-rotate-time": next,
			}},
		}}
		return ops, nil
	}
	return st.db().Run(buildTxn)
}

// WatchSecretsRotationChanges returns a watcher for rotation updates to secrets
// with the specified owner.
func (st *State) WatchSecretsRotationChanges(ownerTags []names.Tag) (SecretsTriggerWatcher, error) {
	if len(ownerTags) == 0 {
		return nil, errors.New("missing secret owners")
	}
	owners := make([]string, len(ownerTags))
	for i, owner := range ownerTags {
		owners[i] = owner.String()
	}
	return newSecretsRotationWatcher(st, owners), nil
}

// SecretsTriggerWatcher defines a watcher for changes to secret
// event trigger config.
type SecretsTriggerWatcher interface {
	Watcher
	Changes() corewatcher.SecretTriggerChannel
}

type rotateWatcherDetails struct {
	txnRevNo int64
	URI      *secrets.URI
}

type secretsRotationWatcher struct {
	commonWatcher
	out chan []corewatcher.SecretTriggerChange

	owners []string
	known  map[string]rotateWatcherDetails
}

func newSecretsRotationWatcher(backend modelBackend, owners []string) *secretsRotationWatcher {
	w := &secretsRotationWatcher{
		commonWatcher: newCommonWatcher(backend),
		out:           make(chan []corewatcher.SecretTriggerChange),
		known:         make(map[string]rotateWatcherDetails),
		owners:        owners,
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

// Changes returns a channel that will receive changes when the next rotate time
// for a secret changes.
func (w *secretsRotationWatcher) Changes() corewatcher.SecretTriggerChannel {
	return w.out
}

func (w *secretsRotationWatcher) initial() ([]corewatcher.SecretTriggerChange, error) {
	var details []corewatcher.SecretTriggerChange

	var doc secretRotationDoc
	secretRotateCollection, closer := w.db.GetCollection(secretRotateC)
	defer closer()

	iter := secretRotateCollection.Find(bson.D{secretOwnerTerm(w.owners)}).Iter()
	for iter.Next(&doc) {
		uriStr := w.backend.localID(doc.DocID)
		uri, err := secrets.ParseURI(uriStr)
		if err != nil {
			_ = iter.Close()
			return nil, errors.Annotatef(err, "invalid secret URI %q", uriStr)
		}
		w.known[doc.DocID] = rotateWatcherDetails{
			txnRevNo: doc.TxnRevno,
			URI:      uri,
		}
		details = append(details, corewatcher.SecretTriggerChange{
			URI:             uri,
			NextTriggerTime: doc.NextRotateTime.UTC(),
		})
	}
	return details, errors.Trace(iter.Close())
}

func (w *secretsRotationWatcher) merge(details []corewatcher.SecretTriggerChange, change watcher.Change) ([]corewatcher.SecretTriggerChange, error) {
	changeID := change.Id.(string)
	knownDetails, known := w.known[changeID]

	doc := secretRotationDoc{}
	if change.Revno >= 0 {
		// Record added or updated.
		secretsRotationColl, closer := w.db.GetCollection(secretRotateC)
		defer closer()
		err := secretsRotationColl.Find(bson.D{{"_id", change.Id}, secretOwnerTerm(w.owners)}).One(&doc)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		// Changed but no longer in the collection so ignore.
		if err != nil {
			return details, nil
		}
	} else if known {
		// Record deleted.
		delete(w.known, changeID)
		deletedDetails := corewatcher.SecretTriggerChange{
			URI: knownDetails.URI,
		}
		for i, detail := range details {
			if detail.URI.ID == changeID {
				details[i] = deletedDetails
				return details, nil
			}
		}
		details = append(details, deletedDetails)
		return details, nil
	}
	if doc.TxnRevno > knownDetails.txnRevNo {
		uriStr := w.backend.localID(doc.DocID)
		uri, err := secrets.ParseURI(uriStr)
		if err != nil {
			return nil, errors.Annotatef(err, "invalid secret URI %q", uriStr)
		}
		w.known[changeID] = rotateWatcherDetails{
			txnRevNo: doc.TxnRevno,
			URI:      uri,
		}
		details = append(details, corewatcher.SecretTriggerChange{
			URI:             uri,
			NextTriggerTime: doc.NextRotateTime.UTC(),
		})
	}
	return details, nil
}

func (w *secretsRotationWatcher) loop() (err error) {
	ch := make(chan watcher.Change)
	w.watcher.WatchCollection(secretRotateC, ch)
	defer w.watcher.UnwatchCollection(secretRotateC, ch)
	details, err := w.initial()
	if err != nil {
		return err
	}
	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case change, ok := <-ch:
			if !ok {
				return tomb.ErrDying
			}
			if details, err = w.merge(details, change); err != nil {
				return err
			}
			if len(details) > 0 {
				out = w.out
			}
		case out <- details:
			out = nil
			details = nil
		}
	}
}

// WatchSecretRevisionsExpiryChanges returns a watcher for expiry time updates to
// secret revisions with the specified owner.
func (st *State) WatchSecretRevisionsExpiryChanges(ownerTags []names.Tag) (SecretsTriggerWatcher, error) {
	if len(ownerTags) == 0 {
		return nil, errors.New("missing secret owners")
	}
	owners := make([]string, len(ownerTags))
	for i, owner := range ownerTags {
		owners[i] = owner.String()
	}
	return newSecretsExpiryWatcher(st, owners), nil
}

type expiryWatcherDetails struct {
	txnRevNo   int64
	uri        *secrets.URI
	revision   int
	willExpire bool
}

type secretsExpiryWatcher struct {
	commonWatcher
	out chan []corewatcher.SecretTriggerChange

	owners []string
	known  map[string]expiryWatcherDetails
}

func newSecretsExpiryWatcher(backend modelBackend, owners []string) *secretsExpiryWatcher {
	w := &secretsExpiryWatcher{
		commonWatcher: newCommonWatcher(backend),
		out:           make(chan []corewatcher.SecretTriggerChange),
		known:         make(map[string]expiryWatcherDetails),
		owners:        owners,
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

// Changes returns a channel that will receive changes when the next expiry time
// for a secret revision changes.
func (w *secretsExpiryWatcher) Changes() corewatcher.SecretTriggerChannel {
	return w.out
}

func (w *secretsExpiryWatcher) initial() ([]corewatcher.SecretTriggerChange, error) {
	var details []corewatcher.SecretTriggerChange

	var doc secretRevisionDoc
	secretRevisionCollection, closer := w.db.GetCollection(secretRevisionsC)
	defer closer()

	iter := secretRevisionCollection.Find(bson.D{secretOwnerTerm(w.owners)}).Iter()
	for iter.Next(&doc) {
		uriStr, _ := splitSecretRevision(w.backend.localID(doc.DocID))
		uri, err := secrets.ParseURI(uriStr)
		if err != nil {
			_ = iter.Close()
			return nil, errors.Annotatef(err, "invalid secret URI %q", uriStr)
		}
		willExpire := doc.ExpireTime != nil
		w.known[doc.DocID] = expiryWatcherDetails{
			txnRevNo:   doc.TxnRevno,
			uri:        uri,
			revision:   doc.Revision,
			willExpire: willExpire,
		}
		if !willExpire {
			continue
		}
		details = append(details, corewatcher.SecretTriggerChange{
			URI:             uri,
			Revision:        doc.Revision,
			NextTriggerTime: doc.ExpireTime.UTC(),
		})
	}
	return details, errors.Trace(iter.Close())
}

func (w *secretsExpiryWatcher) merge(details []corewatcher.SecretTriggerChange, change watcher.Change) ([]corewatcher.SecretTriggerChange, error) {
	changeID := change.Id.(string)
	knownDetails, known := w.known[changeID]

	doc := secretRevisionDoc{}
	if change.Revno >= 0 {
		secretRevisionCollection, closer := w.db.GetCollection(secretRevisionsC)
		defer closer()
		err := secretRevisionCollection.Find(bson.D{{"_id", change.Id}, secretOwnerTerm(w.owners)}).One(&doc)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		// Changed but no longer in the collection so ignore.
		if err != nil {
			return details, nil
		}
	} else if known {
		// Record deleted.
		delete(w.known, changeID)
		deletedDetails := corewatcher.SecretTriggerChange{
			URI:      knownDetails.uri,
			Revision: knownDetails.revision,
		}
		// If an earlier event was received for the same
		// revision, update what we know here.
		for i, detail := range details {
			if detail.URI.ID == knownDetails.uri.ID {
				if knownDetails.willExpire {
					details[i] = deletedDetails
				} else {
					details = append(details[:i], details[i+1:]...)
				}
				return details, nil
			}
		}
		// Only send an update if a deleted revision was
		// previously going to expire.
		if knownDetails.willExpire {
			details = append(details, deletedDetails)
		}
		return details, nil
	}
	if doc.TxnRevno > knownDetails.txnRevNo {
		uriStr, _ := splitSecretRevision(w.backend.localID(doc.DocID))
		uri, err := secrets.ParseURI(uriStr)
		if err != nil {
			return nil, errors.Annotatef(err, "invalid secret URI %q", uriStr)
		}
		willExpire := doc.ExpireTime != nil
		w.known[changeID] = expiryWatcherDetails{
			txnRevNo:   doc.TxnRevno,
			uri:        uri,
			revision:   doc.Revision,
			willExpire: willExpire,
		}
		// The event we send depends on if the revision is set to expire.
		if willExpire {
			details = append(details, corewatcher.SecretTriggerChange{
				URI:             uri,
				Revision:        doc.Revision,
				NextTriggerTime: doc.ExpireTime.UTC(),
			})
		} else if knownDetails.willExpire {
			details = append(details, corewatcher.SecretTriggerChange{
				URI:      uri,
				Revision: doc.Revision,
			})
		}
	}
	return details, nil
}

func (w *secretsExpiryWatcher) loop() (err error) {
	ch := make(chan watcher.Change)
	w.watcher.WatchCollection(secretRevisionsC, ch)
	defer w.watcher.UnwatchCollection(secretRevisionsC, ch)
	details, err := w.initial()
	if err != nil {
		return err
	}
	out := w.out
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.watcher.Dead():
			return stateWatcherDeadError(w.watcher.Err())
		case change, ok := <-ch:
			if !ok {
				return tomb.ErrDying
			}
			if details, err = w.merge(details, change); err != nil {
				return err
			}
			if len(details) > 0 {
				out = w.out
			}
		case out <- details:
			out = nil
			details = nil
		}
	}
}
