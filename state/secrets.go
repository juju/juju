// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/mongo/utils"
	"github.com/juju/juju/state/watcher"
)

// CreateSecretParams are used to create a secret.
type CreateSecretParams struct {
	UpdateSecretParams

	ProviderLabel string
	Version       int
	Owner         string
}

// UpdateSecretParams are used to update a secret.
type UpdateSecretParams struct {
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

// TODO(wallyworld)
type SecretsFilter struct{}

// SecretsStore instances use mongo as a secrets store.
type SecretsStore interface {
	CreateSecret(*secrets.URI, CreateSecretParams) (*secrets.SecretMetadata, error)
	UpdateSecret(*secrets.URI, UpdateSecretParams) (*secrets.SecretMetadata, error)
	GetSecret(*secrets.URI) (*secrets.SecretMetadata, error)
	GetSecretValue(*secrets.URI, int) (secrets.SecretValue, error)
	ListSecrets(SecretsFilter) ([]*secrets.SecretMetadata, error)
	GetSecretConsumer(*secrets.URI, string) (*secrets.SecretConsumerMetadata, error)
	SaveSecretConsumer(*secrets.URI, string, *secrets.SecretConsumerMetadata) error
}

// NewSecretsStore creates a new mongo backed secrets store.
func NewSecretsStore(st *State) *secretsStore {
	return &secretsStore{st: st}
}

type secretMetadataDoc struct {
	DocID string `bson:"_id"`

	Version    int    `bson:"version"`
	OwnerTag   string `bson:"owner-tag"`
	Provider   string `bson:"provider"`
	ProviderID string `bson:"provider-id"`

	Description string `bson:"description"`
	Label       string `bson:"tags"`
	Revision    int    `bson:"latest-revision"`

	RotatePolicy   string     `bson:"rotate-policy"`
	NextRotateTime *time.Time `bson:"next-rotate-time"`
	ExpireTime     *time.Time `bson:"expire-time"`

	CreateTime time.Time `bson:"create-time"`
	UpdateTime time.Time `bson:"update-time"`
}

type secretRevisionDoc struct {
	DocID string `bson:"_id"`

	Revision   int            `bson:"revision"`
	CreateTime time.Time      `bson:"create-time"`
	ExpireTime time.Time      `bson:"expiry"`
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
	md := &secretMetadataDoc{
		DocID:      uri.ID,
		Version:    p.Version,
		OwnerTag:   p.Owner,
		Provider:   p.ProviderLabel,
		ProviderID: "",
		CreateTime: s.st.nowToTheSecond(),
		UpdateTime: s.st.nowToTheSecond(),
	}
	err := s.updateSecretMetadataDoc(md, &p.UpdateSecretParams)
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
	if p.NextRotateTime != nil {
		doc.NextRotateTime = ptr(toValue(p.NextRotateTime).Round(time.Second).UTC())
	}
	if p.ExpireTime != nil {
		doc.ExpireTime = ptr(toValue(p.ExpireTime).Round(time.Second).UTC())
	}
	if len(p.Data) > 0 {
		doc.Revision++
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
	doc := &secretRevisionDoc{
		DocID:      secretRevisionKey(uri, revision),
		Revision:   revision,
		CreateTime: s.st.nowToTheSecond(),
		Data:       dataCopy,
	}
	if expireTime != nil {
		doc.ExpireTime = toValue(expireTime).Round(time.Second).UTC()
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
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if _, err := s.GetSecretValue(uri, revision); err == nil {
				return nil, errors.AlreadyExistsf("secret value for %q", uri.String())
			}
		}
		ops := []txn.Op{
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
			},
		}
		rotateOps, err := s.secretRotationOps(uri, metadataDoc.OwnerTag, p.RotatePolicy, p.NextRotateTime)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, rotateOps...)
		return ops, nil
	}
	err = s.st.db().Run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.toSecretMetadata(metadataDoc)
}

// UpdateSecret updates an existing secret.
func (s *secretsStore) UpdateSecret(uri *secrets.URI, p UpdateSecretParams) (*secrets.SecretMetadata, error) {
	if !p.hasUpdate() {
		return nil, errors.New("must specify a new value or metadata to update a secret")
	}
	secretMetadataCollection, closer := s.st.db().GetCollection(secretMetadataC)
	defer closer()

	var metadataDoc secretMetadataDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		err := secretMetadataCollection.FindId(uri.ID).One(&metadataDoc)
		if errors.Cause(err) == mgo.ErrNotFound {
			return nil, errors.NotFoundf("secret %q", uri.String())
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := s.updateSecretMetadataDoc(&metadataDoc, &p); err != nil {
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		ops := []txn.Op{
			{
				C:      secretMetadataC,
				Id:     metadataDoc.DocID,
				Assert: txn.DocExists,
				Update: bson.M{"$set": metadataDoc},
			},
		}
		if len(p.Data) > 0 {
			if _, err := s.GetSecretValue(uri, metadataDoc.Revision); err == nil {
				return nil, errors.AlreadyExistsf("secret value for %q", uri.String())
			}
			revisionDoc := s.secretRevisionDoc(uri, metadataDoc.Revision, p.ExpireTime, p.Data)
			ops = append(ops, txn.Op{
				C:      secretRevisionsC,
				Id:     revisionDoc.DocID,
				Assert: txn.DocMissing,
				Insert: *revisionDoc,
			})
		}
		if p.RotatePolicy != nil {
			rotateOps, err := s.secretRotationOps(uri, metadataDoc.OwnerTag, p.RotatePolicy, p.NextRotateTime)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, rotateOps...)
		}
		return ops, nil
	}
	err := s.st.db().Run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.toSecretMetadata(&metadataDoc)
}

func (s *secretsStore) toSecretMetadata(doc *secretMetadataDoc) (*secrets.SecretMetadata, error) {
	uri, err := secrets.ParseURI(doc.DocID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	uri.ControllerUUID = s.st.ControllerUUID()
	return &secrets.SecretMetadata{
		URI:            uri,
		Version:        doc.Version,
		RotatePolicy:   secrets.RotatePolicy(doc.RotatePolicy),
		NextRotateTime: doc.NextRotateTime,
		ExpireTime:     doc.ExpireTime,
		Description:    doc.Description,
		Label:          doc.Label,
		OwnerTag:       doc.OwnerTag,
		Provider:       doc.Provider,
		ProviderID:     doc.ProviderID,
		Revision:       doc.Revision,
		CreateTime:     doc.CreateTime,
		UpdateTime:     doc.UpdateTime,
	}, nil
}

// GetSecretValue gets the secret value for the specified URL.
func (s *secretsStore) GetSecretValue(uri *secrets.URI, revision int) (secrets.SecretValue, error) {
	secretValuesCollection, closer := s.st.db().GetCollection(secretRevisionsC)
	defer closer()

	var doc secretRevisionDoc
	err := secretValuesCollection.FindId(secretRevisionKey(uri, revision)).One(&doc)
	if errors.Cause(err) == mgo.ErrNotFound {
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
	if errors.Cause(err) == mgo.ErrNotFound {
		return nil, errors.NotFoundf("secret %q", uri.String())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.toSecretMetadata(&doc)
}

// ListSecrets list the secrets using the specified filter.
// TODO(wallywolrd) - implement filter
func (s *secretsStore) ListSecrets(filter SecretsFilter) ([]*secrets.SecretMetadata, error) {
	secretMetadataCollection, closer := s.st.db().GetCollection(secretMetadataC)
	defer closer()

	var docs []secretMetadataDoc
	// TODO(wallywolrd) - use filter
	err := secretMetadataCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]*secrets.SecretMetadata, len(docs))
	for i, doc := range docs {
		result[i], err = s.toSecretMetadata(&doc)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return result, nil
}

type secretConsumerDoc struct {
	DocID string `bson:"_id"`

	ConsumerTag string `bson:"consumer-tag"`
	Label       string `bson:"label"`
	Revision    int    `bson:"revision"`
}

func secretConsumerKey(id, consumer string) string {
	return fmt.Sprintf("secret#%s#%s", id, consumer)
}

// GetSecretConsumer gets secret consumer metadata.
func (s *secretsStore) GetSecretConsumer(uri *secrets.URI, consumerTag string) (*secrets.SecretConsumerMetadata, error) {
	secretConsumersCollection, closer := s.st.db().GetCollection(secretConsumersC)
	defer closer()

	key := secretConsumerKey(uri.ID, consumerTag)
	docID := s.st.docID(key)
	var doc secretConsumerDoc
	err := secretConsumersCollection.FindId(docID).One(&doc)
	if errors.Cause(err) == mgo.ErrNotFound {
		return nil, errors.NotFoundf("consumer %q metadata for secret %q", consumerTag, uri.String())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &secrets.SecretConsumerMetadata{
		Label:    doc.Label,
		Revision: doc.Revision,
	}, nil
}

// SaveSecretConsumer saves or updates secret consumer metadata.
func (s *secretsStore) SaveSecretConsumer(uri *secrets.URI, consumerTag string, metadata *secrets.SecretConsumerMetadata) error {
	key := secretConsumerKey(uri.ID, consumerTag)
	docID := s.st.docID(key)
	secretConsumersCollection, closer := s.st.db().GetCollection(secretConsumersC)
	defer closer()

	var doc secretConsumerDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		err := secretConsumersCollection.FindId(docID).One(&doc)
		if err := errors.Cause(err); err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		if err == nil {
			return []txn.Op{{
				C:      secretConsumersC,
				Id:     docID,
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{
					"label":    metadata.Label,
					"revision": metadata.Revision,
				}},
			}}, nil
		}
		return []txn.Op{{
			C:      secretConsumersC,
			Id:     docID,
			Assert: txn.DocMissing,
			Insert: secretConsumerDoc{
				DocID:       docID,
				ConsumerTag: consumerTag,
				Label:       metadata.Label,
				Revision:    metadata.Revision,
			},
		}}, nil
	}
	return s.st.db().Run(buildTxn)
}

type secretRotationDoc struct {
	DocID    string `bson:"_id"`
	TxnRevno int64  `bson:"txn-revno"`

	LastRotateTime time.Time `bson:"last-rotate-time"`

	// These fields are denormalised here so that the watcher
	// only needs to access this collection.
	NextRotateTime time.Time `bson:"next-rotate-time"`
	Owner          string    `bson:"owner-tag"`
}

func (s *secretsStore) secretRotationOps(uri *secrets.URI, owner string, rotatePolicy *secrets.RotatePolicy, nextRotateTime *time.Time) ([]txn.Op, error) {
	secretKey := uri.ID
	if p := toValue(rotatePolicy); p == "" || p == secrets.RotateNever {
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
	if err := errors.Cause(err); err != nil && err != mgo.ErrNotFound {
		return nil, errors.Trace(err)
	}
	if err == nil {
		return []txn.Op{{
			C:      secretRotateC,
			Id:     secretKey,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"rotate-time": nextRotateTime}},
		}}, nil
	}
	return []txn.Op{{
		C:      secretRotateC,
		Id:     secretKey,
		Assert: txn.DocMissing,
		Insert: secretRotationDoc{
			DocID:          secretKey,
			NextRotateTime: (*nextRotateTime).Round(time.Second).UTC(),
			Owner:          owner,
			LastRotateTime: s.st.nowToTheSecond().UTC(),
		},
	}}, nil
}

// SecretRotated records when the given secret was rotated.
func (st *State) SecretRotated(uri *secrets.URI, when time.Time) error {
	secretMetadataCollection, closer := st.db().GetCollection(secretMetadataC)
	defer closer()

	secretRotateCollection, closer2 := st.db().GetCollection(secretRotateC)
	defer closer2()

	when = when.Round(time.Second).UTC()
	secretKey := uri.ID
	buildTxn := func(attempt int) ([]txn.Op, error) {
		var metadataDoc secretMetadataDoc
		err := secretMetadataCollection.FindId(secretKey).One(&metadataDoc)
		if errors.Cause(err) == mgo.ErrNotFound {
			return nil, errors.NotFoundf("secret %q", uri.String())
		}
		if err != nil {
			return nil, errors.Trace(err)
		}

		var currentDoc secretRotationDoc
		err = secretRotateCollection.FindId(secretKey).One(&currentDoc)
		if errors.Cause(err) == mgo.ErrNotFound {
			return nil, errors.NotFoundf("rotation info for secret %q", uri.String())
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		// If the secret has already been rotated for a time after what we
		// are trying to set here, keep the newer time.
		if attempt > 0 && currentDoc.LastRotateTime.After(when) {
			return nil, jujutxn.ErrNoOperations
		}
		ops := []txn.Op{{
			C:      secretRotateC,
			Id:     secretKey,
			Assert: bson.D{{"txn-revno", currentDoc.TxnRevno}},
			Update: bson.M{"$set": bson.M{"last-rotate-time": when}},
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
	Changes() corewatcher.SecretRotationChannel
}

type rotateWatcherDetails struct {
	txnRevNo int64
	URI      *secrets.URI
}

type secretsRotationWatcher struct {
	commonWatcher
	out chan []corewatcher.SecretRotationChange

	owner string
	known map[string]rotateWatcherDetails
}

func newSecretsRotationWatcher(backend modelBackend, owner string) *secretsRotationWatcher {
	w := &secretsRotationWatcher{
		commonWatcher: newCommonWatcher(backend),
		out:           make(chan []corewatcher.SecretRotationChange),
		known:         make(map[string]rotateWatcherDetails),
		owner:         owner,
	}
	w.tomb.Go(func() error {
		defer close(w.out)
		return w.loop()
	})
	return w
}

// Changes returns a channel that will receive changes when units enter and
// leave a relation scope. The Entered field in the first event on the channel
// holds the initial state.
func (w *secretsRotationWatcher) Changes() corewatcher.SecretRotationChannel {
	return w.out
}

func (w *secretsRotationWatcher) initial() ([]corewatcher.SecretRotationChange, error) {
	var details []corewatcher.SecretRotationChange

	var doc secretRotationDoc
	secretRotateCollection, closer := w.db.GetCollection(secretRotateC)
	defer closer()

	iter := secretRotateCollection.Find(bson.D{{"owner-tag", w.owner}}).Iter()
	for iter.Next(&doc) {
		uriStr := doc.DocID
		uri, err := secrets.ParseURI(uriStr)
		if err != nil {
			_ = iter.Close()
			return nil, errors.Annotatef(err, "invalid secret URI %q", uriStr)
		}
		w.known[doc.DocID] = rotateWatcherDetails{
			txnRevNo: doc.TxnRevno,
			URI:      uri,
		}
		details = append(details, corewatcher.SecretRotationChange{
			URI: uri,
			// TODO(wallyworld) - fix rotation to work with rotate policy
			RotateInterval: 0,
			LastRotateTime: doc.LastRotateTime.UTC(),
		})
	}
	return details, errors.Trace(iter.Close())
}

func (w *secretsRotationWatcher) merge(details []corewatcher.SecretRotationChange, change watcher.Change) ([]corewatcher.SecretRotationChange, error) {
	changeID := change.Id.(string)
	knownDetails, known := w.known[changeID]

	doc := secretRotationDoc{}
	if change.Revno >= 0 {
		secretsRotationColl, closer := w.db.GetCollection(secretRotateC)
		defer closer()
		err := secretsRotationColl.Find(bson.D{{"_id", change.Id}, {"owner-tag", w.owner}}).One(&doc)
		if err != nil && errors.Cause(err) != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		if err != nil {
			return details, nil
		}
	} else if known {
		for i, detail := range details {
			if detail.URI.ID == changeID {
				details[i].RotateInterval = 0
				return details, nil
			}
		}
		details = append(details, corewatcher.SecretRotationChange{
			URI:            knownDetails.URI,
			RotateInterval: 0,
		})
		return details, nil
	}
	if doc.TxnRevno > knownDetails.txnRevNo {
		uriStr := doc.DocID
		uri, err := secrets.ParseURI(uriStr)
		if err != nil {
			return nil, errors.Annotatef(err, "invalid secret URI %q", uriStr)
		}
		w.known[changeID] = rotateWatcherDetails{
			txnRevNo: doc.TxnRevno,
			URI:      uri,
		}
		details = append(details, corewatcher.SecretRotationChange{
			URI: uri,
			// TODO(wallyworld) - fix rotation to work with rotate policy
			RotateInterval: 0,
			LastRotateTime: doc.LastRotateTime.UTC(),
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
