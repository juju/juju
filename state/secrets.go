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
	"github.com/juju/juju/mongo/utils"
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
	ProviderId     *string
}

func (u *UpdateSecretParams) hasUpdate() bool {
	return u.NextRotateTime != nil ||
		u.RotatePolicy != nil ||
		u.Description != nil ||
		u.Label != nil ||
		u.ExpireTime != nil ||
		len(u.Data) > 0 ||
		u.ProviderId != nil ||
		len(u.Params) > 0
}

// SecretsFilter holds attributes to match when listing secrets.
type SecretsFilter struct {
	URI          *secrets.URI
	OwnerTags    []names.Tag
	ConsumerTags []names.Tag
}

// SecretsStore instances use mongo as a secrets store.
type SecretsStore interface {
	CreateSecret(*secrets.URI, CreateSecretParams) (*secrets.SecretMetadata, error)
	UpdateSecret(*secrets.URI, UpdateSecretParams) (*secrets.SecretMetadata, error)
	DeleteSecret(*secrets.URI, ...int) (bool, error)
	GetSecret(*secrets.URI) (*secrets.SecretMetadata, error)
	GetSecretValue(*secrets.URI, int) (secrets.SecretValue, *string, error)
	ListSecrets(SecretsFilter) ([]*secrets.SecretMetadata, error)
	ListSecretRevisions(uri *secrets.URI) ([]*secrets.SecretRevisionMetadata, error)
	GetSecretRevision(uri *secrets.URI, revision int) (*secrets.SecretRevisionMetadata, error)
	WatchObsolete(owners []names.Tag) (StringsWatcher, error)
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
	ProviderId *string        `bson:"provider-id,omitempty"`

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
	if p.RotatePolicy != nil {
		doc.RotatePolicy = string(toValue(p.RotatePolicy))
	}
	hasData := len(p.Data) > 0 || p.ProviderId != nil
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

func (s *secretsStore) secretRevisionDoc(uri *secrets.URI, owner string, revision int, expireTime *time.Time, data secrets.SecretData, providerId *string) *secretRevisionDoc {
	dataCopy := make(secretsDataMap)
	for k, v := range data {
		dataCopy[k] = v
	}
	now := s.st.nowToTheSecond()
	doc := &secretRevisionDoc{
		DocID:      secretRevisionKey(uri, revision),
		Revision:   revision,
		OwnerTag:   owner,
		CreateTime: now,
		UpdateTime: now,
		Data:       dataCopy,
		ProviderId: providerId,
	}
	if expireTime != nil {
		expire := expireTime.Round(time.Second).UTC()
		doc.ExpireTime = &expire
	}
	return doc
}

// CreateSecret creates a new secret.
func (s *secretsStore) CreateSecret(uri *secrets.URI, p CreateSecretParams) (*secrets.SecretMetadata, error) {
	if len(p.Data) == 0 && p.ProviderId == nil {
		return nil, errors.New("cannot create a secret without content")
	}
	metadataDoc, err := s.secretMetadataDoc(uri, &p)
	if err != nil {
		return nil, errors.Trace(err)
	}
	revision := 1
	valueDoc := s.secretRevisionDoc(uri, p.Owner.String(), revision, p.ExpireTime, p.Data, p.ProviderId)
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
			uniqueLabelOps, err := s.st.uniqueSecretLabelOps(metadataDoc.OwnerTag, *p.Label)
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
			uniqueLabelOps, err := s.st.uniqueSecretLabelOps(metadataDoc.OwnerTag, *p.Label)
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
		if !revisionExists && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if len(p.Data) > 0 || p.ProviderId != nil {
			if revisionExists {
				return nil, errors.AlreadyExistsf("secret value with revision %d for %q", metadataDoc.LatestRevision, uri.String())
			}
			revisionDoc := s.secretRevisionDoc(uri, metadataDoc.OwnerTag, metadataDoc.LatestRevision, newExpireTime, p.Data, p.ProviderId)
			ops = append(ops, txn.Op{
				C:      secretRevisionsC,
				Id:     revisionDoc.DocID,
				Assert: txn.DocMissing,
				Insert: *revisionDoc,
			})
			// Ensure no new consumers are added while update is in progress.
			countOps, err := s.st.checkConsumerCountOps(uri, 0)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, countOps...)

			updateConsumersOps, err := s.st.secretUpdateConsumersOps(uri, metadataDoc.LatestRevision)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, updateConsumersOps...)

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
		CreateTime:       doc.CreateTime,
		UpdateTime:       doc.UpdateTime,
	}, nil
}

// DeleteSecret deletes the specified secret revisions.
// If revisions is nil or the last remaining revisions are
// removed, the entire secret is deleted and the return bool is true.
func (s *secretsStore) DeleteSecret(uri *secrets.URI, revisions ...int) (removed bool, err error) {
	return s.st.deleteSecrets([]*secrets.URI{uri}, revisions...)
}

func (st *State) deleteSecrets(uris []*secrets.URI, revisions ...int) (removed bool, err error) {
	// We will bulk delete the various artefacts, starting with the secret itself.
	// Deleting the parent secret metadata first will ensure that any consumers of
	// the secret get notified and subsequent attempts to access any secret
	// attributes (revision etc) return not found.
	// It is not practical to do this record by record in a legacy client side mgo txn operation.
	if len(uris) == 0 && len(revisions) == 0 {
		// Nothing to remove.
		return false, nil
	}

	if len(uris) == 0 || len(uris) > 1 && len(revisions) > 0 {
		return false, errors.Errorf("PROGRAMMING ERROR: invalid secret deletion args uris=%v, revisions=%v", uris, revisions)
	}
	session := st.MongoSession()
	err = session.StartTransaction()
	if err != nil {
		return false, errors.Trace(err)
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
	secretRevisionsCollection, closer := st.db().GetCollection(secretRevisionsC)
	defer closer()

	if len(revisions) > 0 {
		uri := uris[0]
		var savedRevisionDocs []secretRevisionDoc
		err := secretRevisionsCollection.Find(bson.D{{"_id",
			bson.D{{"$regex", fmt.Sprintf("%s/.*", uri.ID)}}}}).Select(bson.D{{"revision", 1}}).All(&savedRevisionDocs)
		if err != nil {
			return false, errors.Annotatef(err, "counting revisions for %s", uri.String())
		}
		toDelete := set.NewInts(revisions...)
		savedRevisions := set.NewInts()
		for _, r := range savedRevisionDocs {
			savedRevisions.Add(r.Revision)
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
			return false, errors.Annotatef(err, "deleting revisions for %s", uri.String())
		}
	}

	secretMetadataCollection, closer := st.db().GetCollection(secretMetadataC)
	defer closer()

	deleteOne := func(uri *secrets.URI) (bool, error) {
		var md secretMetadataDoc
		err = secretMetadataCollection.FindId(uri.ID).One(&md)
		if err == mgo.ErrNotFound {
			return true, nil
		}
		if err != nil {
			return false, errors.Trace(err)
		}
		_, err = secretMetadataCollection.Writeable().RemoveAll(bson.D{{
			"_id", uri.ID,
		}})
		if err != nil {
			return false, errors.Annotatef(err, "deleting revisions for %s", uri.String())
		}

		secretRotateCollection, closer := st.db().GetCollection(secretRotateC)
		defer closer()
		_, err = secretRotateCollection.Writeable().RemoveAll(bson.D{{
			"_id", uri.ID,
		}})
		if err != nil {
			return false, errors.Annotatef(err, "deleting revisions for %s", uri.String())
		}

		_, err = secretRevisionsCollection.Writeable().RemoveAll(bson.D{{
			"_id", bson.D{{"$regex", fmt.Sprintf("%s/.*", uri.ID)}},
		}})
		if err != nil {
			return false, errors.Annotatef(err, "deleting revisions for %s", uri.String())
		}

		secretPermissionsCollection, closer := st.db().GetCollection(secretPermissionsC)
		defer closer()
		_, err = secretPermissionsCollection.Writeable().RemoveAll(bson.D{{
			"_id", bson.D{{"$regex", fmt.Sprintf("%s#.*", uri.ID)}},
		}})
		if err != nil {
			return false, errors.Annotatef(err, "deleting permissions for %s", uri.String())
		}

		secretConsumersCollection, closer := st.db().GetCollection(secretConsumersC)
		defer closer()
		_, err = secretConsumersCollection.Writeable().RemoveAll(bson.D{{
			"_id", bson.D{{"$regex", fmt.Sprintf("%s#.*", uri.ID)}},
		}})
		if err != nil {
			return false, errors.Annotatef(err, "deleting consumer info for %s", uri.String())
		}
		refCountsCollection, closer := st.db().GetCollection(refcountsC)
		defer closer()
		_, err = refCountsCollection.Writeable().RemoveAll(bson.D{{
			"_id", fmt.Sprintf("%s#%s", uri.ID, "consumer"),
		}})
		if err != nil {
			return false, errors.Annotatef(err, "deleting consumer refcounts for %s", uri.String())
		}
		_, err = refCountsCollection.Writeable().RemoveAll(bson.D{{
			"_id", secretOwnerLabelKey(md.OwnerTag, md.Label),
		}})
		if err != nil {
			return false, errors.Annotatef(err, "deleting label refcounts for %s", uri.String())
		}
		return true, nil
	}
	anyRemoved := false
	for _, uri := range uris {
		removed, err := deleteOne(uri)
		if err != nil {
			return false, errors.Annotatef(err, "deleting secret %q", uri.String())
		}
		anyRemoved = anyRemoved || removed
	}
	return anyRemoved, nil
}

// GetSecretValue gets the secret value for the specified URL.
func (s *secretsStore) GetSecretValue(uri *secrets.URI, revision int) (secrets.SecretValue, *string, error) {
	return s.getSecretValue(uri, revision, true)
}

func (s *secretsStore) getSecretValue(uri *secrets.URI, revision int, checkExists bool) (secrets.SecretValue, *string, error) {
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
	return secrets.NewSecretValue(data), doc.ProviderId, nil
}

// GetSecret gets the secret metadata for the specified URL.
func (s *secretsStore) GetSecret(uri *secrets.URI) (*secrets.SecretMetadata, error) {
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

func (s *secretsStore) listConsumedSecrets(consumers []string) ([]string, error) {
	secretPermissionsCollection, closer := s.st.db().GetCollection(secretPermissionsC)
	defer closer()
	var docs []secretPermissionDoc
	err := secretPermissionsCollection.Find(bson.D{
		{"_id", bson.D{{"$regex", fmt.Sprintf(".*#(%s)", strings.Join(consumers, "|"))}}},
		{"role", secrets.RoleView},
	}).All(&docs)
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

func (s *secretsStore) listSecretRevisions(uri *secrets.URI, revision *int) ([]*secrets.SecretRevisionMetadata, error) {
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
	result := make([]*secrets.SecretRevisionMetadata, len(docs))
	for i, doc := range docs {
		result[i] = &secrets.SecretRevisionMetadata{
			Revision:   doc.Revision,
			ProviderId: doc.ProviderId,
			CreateTime: doc.CreateTime,
			UpdateTime: doc.UpdateTime,
			ExpireTime: doc.ExpireTime,
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

func secretConsumerKey(id, consumer string) string {
	return fmt.Sprintf("%s#%s", id, consumer)
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

func secretOwnerLabelKey(ownerTag string, label string) string {
	return fmt.Sprintf("secretlabel#%s#%s", ownerTag, label)
}

func (st *State) uniqueSecretLabelOps(ownerTag string, label string) ([]txn.Op, error) {
	refCountCollection, ccloser := st.db().GetCollection(refcountsC)
	defer ccloser()

	key := secretOwnerLabelKey(ownerTag, label)
	countOp, count, err := nsRefcounts.CurrentOp(refCountCollection, key)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if count > 0 {
		return nil, errors.WithType(
			errors.Errorf("secret label %q owned by %s already exists", label, ownerTag), LabelExists)
	}
	incOp, err := nsRefcounts.CreateOrIncRefOp(refCountCollection, key, 1)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return []txn.Op{countOp, incOp}, nil
}

func (st *State) removeOwnerSecretLabelOps(ownerTag names.Tag) ([]txn.Op, error) {
	refCountsCollection, closer := st.db().GetCollection(refcountsC)
	defer closer()

	var (
		doc bson.M
		ops []txn.Op
	)
	id := secretOwnerLabelKey(ownerTag.String(), ".*")
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

// GetSecretConsumer gets secret consumer metadata.
func (st *State) GetSecretConsumer(uri *secrets.URI, consumer names.Tag) (*secrets.SecretConsumerMetadata, error) {
	if err := st.checkExists(uri); err != nil {
		return nil, errors.Trace(err)
	}

	secretConsumersCollection, closer := st.db().GetCollection(secretConsumersC)
	defer closer()

	key := secretConsumerKey(uri.ID, consumer.String())
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

// SaveSecretConsumer saves or updates secret consumer metadata.
func (st *State) SaveSecretConsumer(uri *secrets.URI, consumer names.Tag, metadata *secrets.SecretConsumerMetadata) error {
	key := secretConsumerKey(uri.ID, consumer.String())
	secretConsumersCollection, closer := st.db().GetCollection(secretConsumersC)
	defer closer()

	var doc secretConsumerDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if err := st.checkExists(uri); err != nil {
			return nil, errors.Trace(err)
		}
		var ops []txn.Op
		err := secretConsumersCollection.FindId(key).One(&doc)
		if err == nil {
			ops = []txn.Op{{
				C:      secretConsumersC,
				Id:     key,
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{
					"label":            metadata.Label,
					"current-revision": metadata.CurrentRevision,
				}},
			}}
		} else if err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		} else {
			ops = []txn.Op{{
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
			}}

			// Increment the consumer count, ensuring no new consumers
			// are added while update is in progress.
			countRefOps, err := st.checkConsumerCountOps(uri, 1)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, countRefOps...)
		}

		// The consumer is tracking a new revision, which might result in the
		// previous revision becoming obsolete.
		obsoleteOps, err := st.markObsoleteRevisionOps(uri, consumer.String(), metadata.CurrentRevision)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, obsoleteOps...)

		return ops, nil
	}
	return st.db().Run(buildTxn)
}

// secretUpdateConsumersOps updates the latest secret revision number
// on all consumers. This triggers the secrets change watcher.
func (st *State) secretUpdateConsumersOps(uri *secrets.URI, newRevision int) ([]txn.Op, error) {
	secretConsumersCollection, closer := st.db().GetCollection(secretConsumersC)
	defer closer()

	var (
		doc secretConsumerDoc
		ops []txn.Op
	)
	id := secretConsumerKey(uri.ID, ".*")
	q := bson.D{{"_id", bson.D{{"$regex", id}}}}
	iter := secretConsumersCollection.Find(q).Iter()
	for iter.Next(&doc) {
		ops = append(ops, txn.Op{
			C:      secretConsumersC,
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

// allSecretConsumers is used for model export.
func (s *secretsStore) allSecretConsumers() ([]secretConsumerDoc, error) {
	secretConsumerCollection, closer := s.st.db().GetCollection(secretConsumersC)
	defer closer()

	var docs []secretConsumerDoc

	err := secretConsumerCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

// WatchConsumedSecretsChanges returns a watcher for updates and deletes
// of secrets that have been previously read by the specified consumer.
func (st *State) WatchConsumedSecretsChanges(consumer names.Tag) (StringsWatcher, error) {
	return newConsumedSecretsWatcher(st, consumer.String()), nil
}

type consumedSecretsWatcher struct {
	commonWatcher
	out chan []string

	consumer       string
	knownRevisions map[string]int
}

func newConsumedSecretsWatcher(st modelBackend, consumer string) StringsWatcher {
	w := &consumedSecretsWatcher{
		commonWatcher:  newCommonWatcher(st),
		out:            make(chan []string),
		knownRevisions: make(map[string]int),
		consumer:       consumer,
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

func (w *consumedSecretsWatcher) initial() error {
	var doc secretConsumerDoc
	secretConsumersCollection, closer := w.db.GetCollection(secretConsumersC)
	defer closer()

	iter := secretConsumersCollection.Find(bson.D{{"consumer-tag", w.consumer}}).Iter()
	for iter.Next(&doc) {
		w.knownRevisions[doc.DocID] = doc.LatestRevision
	}
	return errors.Trace(iter.Close())
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
	secretConsumerColl, closer := w.db.GetCollection(secretConsumersC)
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
		return strings.HasSuffix(k, "#"+w.consumer)
	}
	w.watcher.WatchCollectionWithFilter(secretConsumersC, ch, filter)
	defer w.watcher.UnwatchCollection(secretConsumersC, ch)

	if err = w.initial(); err != nil {
		return errors.Trace(err)
	}

	var changes []string
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
	return newObsoleteSecretsWatcher(s.st, owners), nil
}

type obsoleteSecretsWatcher struct {
	commonWatcher
	out chan []string

	obsoleteRevisionsWatcher *collectionWatcher

	owners []string
	known  set.Strings
}

func newObsoleteSecretsWatcher(st modelBackend, owners []string) *obsoleteSecretsWatcher {
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

			return doc.Obsolete
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

func splitSecretRevision(c string) (string, int) {
	parts := strings.Split(c, "/")
	if len(parts) < 2 {
		return parts[0], 0
	}
	rev, _ := strconv.Atoi(parts[1])
	return parts[0], rev
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

	secretConsumersCollection, closer := st.db().GetCollection(secretConsumersC)
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
	err = pipe.All(&usedRevisions)
	if err != nil {
		return nil, 0, errors.Trace(err)
	}
	if len(usedRevisions) == 0 {
		return allRevisions.Values(), latest, nil
	}

	for revStr := range usedRevisions[0] {
		r, _ := strconv.Atoi(revStr)
		allRevisions.Remove(r)
	}
	return allRevisions.Values(), latest, nil
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
		} else if errors.IsNotFound(err) {
			entity, err = st.RemoteApplication(id)
			collName = remoteApplicationsC
		}
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
	if err != nil {
		return errors.Annotate(err, "invalid subject reference")
	}
	if subjectEntity.Life() != Alive {
		return errors.Errorf("cannot grant dying %q access to secret", p.Subject)
	}
	isCrossModel := subjectCollName == remoteApplicationsC
	if subjectCollName == unitsC {
		unitApp, _ := names.UnitApplication(p.Subject.Id())
		_, err = st.RemoteApplication(unitApp)
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		isCrossModel = err == nil
	}
	if isCrossModel {
		return errors.NotSupportedf("sharing secrets across a cross model relation")
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

	key := secretConsumerKey(uri.ID, p.Subject.String())

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
	key := secretConsumerKey(uri.ID, p.Subject.String())

	secretPermissionsCollection, closer := st.db().GetCollection(secretPermissionsC)
	defer closer()

	var doc secretPermissionDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if err := st.checkExists(uri); err != nil {
			if errors.IsNotFound(err) {
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
	key := secretConsumerKey(uri.ID, subject.String())

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
