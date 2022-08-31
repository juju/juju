// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

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
	Owner   string
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
}

func (u *UpdateSecretParams) hasUpdate() bool {
	return u.NextRotateTime != nil ||
		u.RotatePolicy != nil ||
		u.Description != nil ||
		u.Label != nil ||
		u.ExpireTime != nil ||
		len(u.Data) > 0 ||
		len(u.Params) > 0
}

// SecretsFilter holds attributes to match when listing secrets.
type SecretsFilter struct {
	URI      *secrets.URI
	OwnerTag *string
}

// SecretsStore instances use mongo as a secrets store.
type SecretsStore interface {
	CreateSecret(*secrets.URI, CreateSecretParams) (*secrets.SecretMetadata, error)
	UpdateSecret(*secrets.URI, UpdateSecretParams) (*secrets.SecretMetadata, error)
	DeleteSecret(*secrets.URI) error
	GetSecret(*secrets.URI) (*secrets.SecretMetadata, error)
	GetSecretValue(*secrets.URI, int) (secrets.SecretValue, error)
	ListSecrets(SecretsFilter) ([]*secrets.SecretMetadata, error)
	ListSecretRevisions(uri *secrets.URI) ([]*secrets.SecretRevisionMetadata, error)
	GetSecretRevision(uri *secrets.URI, revision int) (*secrets.SecretRevisionMetadata, error)
}

// NewSecrets creates a new mongo backed secrets store.
func NewSecrets(st *State) *secretsStore {
	return &secretsStore{st: st}
}

type secretMetadataDoc struct {
	DocID string `bson:"_id"`

	Version    int    `bson:"version"`
	OwnerTag   string `bson:"owner-tag"`
	ProviderID string `bson:"provider-id"`

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
	DocID string `bson:"_id"`

	Revision   int            `bson:"revision"`
	CreateTime time.Time      `bson:"create-time"`
	UpdateTime time.Time      `bson:"update-time"`
	ExpireTime *time.Time     `bson:"expire-time,omitempty"`
	Data       secretsDataMap `bson:"data"`
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
		OwnerTag:   p.Owner,
		ProviderID: "",
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
	if p.ExpireTime != nil {
		doc.LatestExpireTime = ptr(toValue(p.ExpireTime).Round(time.Second).UTC())
	}
	if len(p.Data) > 0 {
		doc.LatestRevision++
	}
	doc.UpdateTime = s.st.nowToTheSecond()
	return nil
}

func secretRevisionKey(uri *secrets.URI, revision int) string {
	return fmt.Sprintf("%s/%d", uri.ID, revision)
}

func (s *secretsStore) secretRevisionDoc(uri *secrets.URI, revision int, expireTime *time.Time, data secrets.SecretData) *secretRevisionDoc {
	dataCopy := make(secretsDataMap)
	for k, v := range data {
		dataCopy[k] = v
	}
	now := s.st.nowToTheSecond()
	doc := &secretRevisionDoc{
		DocID:      secretRevisionKey(uri, revision),
		Revision:   revision,
		CreateTime: now,
		UpdateTime: now,
		Data:       dataCopy,
	}
	if expireTime != nil {
		expire := expireTime.Round(time.Second).UTC()
		doc.ExpireTime = &expire
	}
	return doc
}

// CreateSecret creates a new secret.
func (s *secretsStore) CreateSecret(uri *secrets.URI, p CreateSecretParams) (*secrets.SecretMetadata, error) {
	metadataDoc, err := s.secretMetadataDoc(uri, &p)
	if err != nil {
		return nil, errors.Trace(err)
	}
	revision := 1
	valueDoc := s.secretRevisionDoc(uri, revision, p.ExpireTime, p.Data)
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
			if _, err := s.getSecretValue(uri, revision, false); err == nil {
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
		return errors.NotFoundf("secret %q", uri.ShortString())
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
		if err := s.updateSecretMetadataDoc(&metadataDoc, &p); err != nil {
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		ops = append(ops, []txn.Op{
			{
				C:      secretMetadataC,
				Id:     metadataDoc.DocID,
				Assert: txn.DocExists,
				Update: bson.M{"$set": metadataDoc},
			},
		}...)
		_, err = s.getSecretValue(uri, metadataDoc.LatestRevision, false)
		revisionExists := err == nil
		if !revisionExists && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if len(p.Data) > 0 {
			if revisionExists {
				return nil, errors.AlreadyExistsf("secret value with revision %d for %q", metadataDoc.LatestRevision, uri.String())
			}
			revisionDoc := s.secretRevisionDoc(uri, metadataDoc.LatestRevision, p.ExpireTime, p.Data)
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
		} else if p.ExpireTime != nil {
			if !revisionExists {
				return nil, errors.NotFoundf("reversion %d for secret %q", metadataDoc.LatestRevision, uri.String())
			}
			ops = append(ops, txn.Op{
				C:      secretRevisionsC,
				Id:     secretRevisionKey(uri, metadataDoc.LatestRevision),
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{"expire-time": p.ExpireTime}},
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
	uri.ControllerUUID = s.st.ControllerUUID()
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
		ProviderID:       doc.ProviderID,
		CreateTime:       doc.CreateTime,
		UpdateTime:       doc.UpdateTime,
	}, nil
}

// DeleteSecret deletes the specified secret.
func (s *secretsStore) DeleteSecret(uri *secrets.URI) (err error) {
	// We will bulk delete the various artefacts, starting with the secret itself.
	// Deleting the parent secret metadata first  will ensure that any consumers of
	// the secret get notified and subsequent attempts to access any secret
	// attributes (revision etc) return not found.
	// It is not practical to do this record by record in a legacy client side mgo txn operation.
	session := s.st.MongoSession()
	err = session.StartTransaction()
	if err != nil {
		return errors.Trace(err)
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
	secretMetadataCollection, closer := s.st.db().GetCollection(secretMetadataC)
	defer closer()
	// We need to read something for the txn to work with RemoveAll.
	var v interface{}
	err = secretMetadataCollection.FindId(uri.ID).One(&v)
	if err == mgo.ErrNotFound {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	_, err = secretMetadataCollection.Writeable().RemoveAll(bson.D{{
		"_id", uri.ID,
	}})
	if err != nil {
		return errors.Annotatef(err, "deleting revisions for %s", uri.ShortString())
	}

	secretRotateCollection, closer := s.st.db().GetCollection(secretRotateC)
	defer closer()
	_, err = secretRotateCollection.Writeable().RemoveAll(bson.D{{
		"_id", uri.ID,
	}})
	if err != nil {
		return errors.Annotatef(err, "deleting revisions for %s", uri.ShortString())
	}

	secretRevisionsCollection, closer := s.st.db().GetCollection(secretRevisionsC)
	defer closer()
	_, err = secretRevisionsCollection.Writeable().RemoveAll(bson.D{{
		"_id", bson.D{{"$regex", fmt.Sprintf("%s/.*", uri.ID)}},
	}})
	if err != nil {
		return errors.Annotatef(err, "deleting revisions for %s", uri.ShortString())
	}

	secretPermissionsCollection, closer := s.st.db().GetCollection(secretPermissionsC)
	defer closer()
	_, err = secretPermissionsCollection.Writeable().RemoveAll(bson.D{{
		"_id", bson.D{{"$regex", fmt.Sprintf("%s#.*", uri.ID)}},
	}})
	if err != nil {
		return errors.Annotatef(err, "deleting permissions for %s", uri.ShortString())
	}

	secretConsumersCollection, closer := s.st.db().GetCollection(secretConsumersC)
	defer closer()
	_, err = secretConsumersCollection.Writeable().RemoveAll(bson.D{{
		"_id", bson.D{{"$regex", fmt.Sprintf("%s#.*", uri.ID)}},
	}})
	if err != nil {
		return errors.Annotatef(err, "deleting consumer info for %s", uri.ShortString())
	}
	refCountsCollection, closer := s.st.db().GetCollection(refcountsC)
	defer closer()
	_, err = refCountsCollection.Writeable().RemoveAll(bson.D{{
		"_id", fmt.Sprintf("%s#%s", uri.ID, "consumer"),
	}})
	if err != nil {
		return errors.Annotatef(err, "deleting consumer info for %s", uri.ShortString())
	}
	return nil
}

// GetSecretValue gets the secret value for the specified URL.
func (s *secretsStore) GetSecretValue(uri *secrets.URI, revision int) (secrets.SecretValue, error) {
	return s.getSecretValue(uri, revision, true)
}

func (s *secretsStore) getSecretValue(uri *secrets.URI, revision int, checkExists bool) (secrets.SecretValue, error) {
	if checkExists {
		if err := s.st.checkExists(uri); err != nil {
			return nil, errors.Trace(err)
		}
	}
	secretValuesCollection, closer := s.st.db().GetCollection(secretRevisionsC)
	defer closer()

	var doc secretRevisionDoc
	err := secretValuesCollection.FindId(secretRevisionKey(uri, revision)).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("secret %q", uri.String())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	data := make(secrets.SecretData)
	for k, v := range doc.Data {
		data[k] = fmt.Sprintf("%v", v)
	}
	return secrets.NewSecretValue(data), nil
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

// ListSecrets list the secrets using the specified filter.
func (s *secretsStore) ListSecrets(filter SecretsFilter) ([]*secrets.SecretMetadata, error) {
	secretMetadataCollection, closer := s.st.db().GetCollection(secretMetadataC)
	defer closer()

	var docs []secretMetadataDoc

	q := bson.D{}
	if filter.URI != nil {
		q = append(q, bson.DocElem{"_id", filter.URI.ID})
	}
	if filter.OwnerTag != nil {
		q = append(q, bson.DocElem{"owner-tag", *filter.OwnerTag})
	}
	err := secretMetadataCollection.Find(q).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
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
	return result, nil
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
	err := secretRevisionCollection.Find(q).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]*secrets.SecretRevisionMetadata, len(docs))
	for i, doc := range docs {
		result[i] = &secrets.SecretRevisionMetadata{
			Revision:   doc.Revision,
			CreateTime: doc.CreateTime,
			UpdateTime: doc.UpdateTime,
			ExpireTime: doc.ExpireTime,
		}
	}
	return result, nil
}

type secretConsumerDoc struct {
	DocID string `bson:"_id"`

	ConsumerTag     string `bson:"consumer-tag"`
	Label           string `bson:"label"`
	CurrentRevision int    `bson:"current-revision"`
	LatestRevision  int    `bson:"latest-revision"`
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
func (st *State) GetSecretConsumer(uri *secrets.URI, consumerTag string) (*secrets.SecretConsumerMetadata, error) {
	if err := st.checkExists(uri); err != nil {
		return nil, errors.Trace(err)
	}

	secretConsumersCollection, closer := st.db().GetCollection(secretConsumersC)
	defer closer()

	key := secretConsumerKey(uri.ID, consumerTag)
	var doc secretConsumerDoc
	err := secretConsumersCollection.FindId(key).One(&doc)
	if errors.Cause(err) == mgo.ErrNotFound {
		return nil, errors.NotFoundf("consumer %q metadata for secret %q", consumerTag, uri.String())
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
func (st *State) SaveSecretConsumer(uri *secrets.URI, consumerTag string, metadata *secrets.SecretConsumerMetadata) error {
	key := secretConsumerKey(uri.ID, consumerTag)
	secretConsumersCollection, closer := st.db().GetCollection(secretConsumersC)
	defer closer()

	var doc secretConsumerDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if err := st.checkExists(uri); err != nil {
			return nil, errors.Trace(err)
		}
		err := secretConsumersCollection.FindId(key).One(&doc)
		if err == nil {
			return []txn.Op{{
				C:      secretConsumersC,
				Id:     key,
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{
					"label":            metadata.Label,
					"current-revision": metadata.CurrentRevision,
				}},
			}}, nil
		} else if err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		ops := []txn.Op{{
			C:      secretConsumersC,
			Id:     key,
			Assert: txn.DocMissing,
			Insert: secretConsumerDoc{
				DocID:           key,
				ConsumerTag:     consumerTag,
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

// WatchConsumedSecretsChanges returns a watcher for updates to secrets
// that have been previously read by the specified consumer.
func (st *State) WatchConsumedSecretsChanges(consumer string) StringsWatcher {
	return newCollectionWatcher(st, colWCfg{
		col: secretConsumersC,
		filter: func(key interface{}) bool {
			secretConsumersCollection, closer := st.db().GetCollection(secretConsumersC)
			defer closer()

			var doc secretConsumerDoc
			err := secretConsumersCollection.FindId(key).One(&doc)
			if err != nil {
				return false
			}
			// Only trigger on revisions > 1 because the initial secret-get
			// will have read revision 1.
			return doc.ConsumerTag == consumer && doc.LatestRevision > 1
		},
		idconv: func(id string) string {
			parts := strings.Split(id, "#")
			if len(parts) < 1 {
				return id
			}
			uri := secrets.URI{ID: parts[0]}
			return uri.String()
		},
	})
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
		collName = applicationsC
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

	entity, scopeCollName, scopeDocID, err := st.findSecretEntity(p.Scope)
	if err != nil {
		return errors.Annotate(err, "invalid scope reference")
	}
	if entity.Life() != Alive {
		return errors.Errorf("cannot grant access to secret in scope of %q which is not alive", p.Scope)
	}
	isScopeAliveOp := txn.Op{
		C:      scopeCollName,
		Id:     scopeDocID,
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
		}, isScopeAliveOp}, nil
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
	Owner          string    `bson:"owner-tag"`
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
				Owner:          owner,
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
func (st *State) WatchSecretsRotationChanges(owner string) SecretsRotationWatcher {
	return newSecretsRotationWatcher(st, owner)
}

// SecretsRotationWatcher defines a watcher for secret rotation config.
type SecretsRotationWatcher interface {
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

	owner string
	known map[string]rotateWatcherDetails
}

func newSecretsRotationWatcher(backend modelBackend, owner string) *secretsRotationWatcher {
	w := &secretsRotationWatcher{
		commonWatcher: newCommonWatcher(backend),
		out:           make(chan []corewatcher.SecretTriggerChange),
		known:         make(map[string]rotateWatcherDetails),
		owner:         owner,
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

	iter := secretRotateCollection.Find(bson.D{{"owner-tag", w.owner}}).Iter()
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
		err := secretsRotationColl.Find(bson.D{{"_id", change.Id}, {"owner-tag", w.owner}}).One(&doc)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		// Chenged but no longer in the collection so ignore.
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
		case change := <-ch:
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
