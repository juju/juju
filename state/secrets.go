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
	jujutxn "github.com/juju/txn/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/mongo/utils"
	"github.com/juju/juju/state/watcher"
)

// CreateSecretParams are used to create a secret.
type CreateSecretParams struct {
	ProviderLabel  string
	Version        int
	Owner          string
	RotateInterval time.Duration
	Description    string
	Tags           map[string]string
	Params         map[string]interface{}
	Data           secrets.SecretData
}

// UpdateSecretParams are used to update a secret.
type UpdateSecretParams struct {
	RotateInterval *time.Duration
	Description    *string
	Tags           *map[string]string
	Params         map[string]interface{}
	Data           secrets.SecretData
}

func (u *UpdateSecretParams) hasUpdate() bool {
	return u.RotateInterval != nil ||
		u.Description != nil ||
		u.Tags != nil ||
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
}

// NewSecretsStore creates a new mongo backed secrets store.
func NewSecretsStore(st *State) *secretsStore {
	return &secretsStore{st: st}
}

type secretMetadataDoc struct {
	DocID string `bson:"_id"`

	Version        int               `bson:"version"`
	OwnerTag       string            `bson:"owner-tag"`
	RotateInterval time.Duration     `bson:"rotate-interval"`
	Description    string            `bson:"description"`
	Tags           map[string]string `bson:"tags"`
	Provider       string            `bson:"provider"`
	ProviderID     string            `bson:"provider-id"`
	Revision       int               `bson:"latest-revision"`
	CreateTime     time.Time         `bson:"create-time"`
	UpdateTime     time.Time         `bson:"update-time"`
}

type secretValueDoc struct {
	DocID string `bson:"_id"`

	Revision   int            `bson:"revision"`
	CreateTime time.Time      `bson:"create-time"`
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

func (s *secretsStore) secretMetadataDoc(uri *secrets.URI, p *CreateSecretParams) (*secretMetadataDoc, error) {
	interval := p.RotateInterval
	if interval < 0 {
		interval = 0
	}
	md := &secretMetadataDoc{
		DocID:          uri.ID,
		Version:        p.Version,
		OwnerTag:       p.Owner,
		RotateInterval: interval,
		Description:    p.Description,
		Tags:           p.Tags,
		Provider:       p.ProviderLabel,
		ProviderID:     "",
		Revision:       1,
		CreateTime:     s.st.nowToTheSecond(),
		UpdateTime:     s.st.nowToTheSecond(),
	}
	return md, nil
}

func (s *secretsStore) updateSecretMetadataDoc(doc *secretMetadataDoc, p *UpdateSecretParams) {
	if p.RotateInterval != nil {
		doc.RotateInterval = *p.RotateInterval
	}
	if p.Description != nil {
		doc.Description = *p.Description
	}
	if p.Tags != nil {
		doc.Tags = *p.Tags
	}
	if len(p.Data) > 0 {
		doc.Revision++
	}
	doc.UpdateTime = s.st.nowToTheSecond()
}

func secretRevisionKey(uri *secrets.URI, revision int) string {
	return fmt.Sprintf("%s/%d", uri.ID, revision)
}

func (s *secretsStore) secretValueDoc(uri *secrets.URI, revision int, data secrets.SecretData) *secretValueDoc {
	dataCopy := make(secretsDataMap)
	for k, v := range data {
		dataCopy[k] = v
	}
	return &secretValueDoc{
		DocID:      secretRevisionKey(uri, revision),
		Revision:   revision,
		CreateTime: s.st.nowToTheSecond(),
		Data:       dataCopy,
	}
}

// CreateSecret creates a new secret.
func (s *secretsStore) CreateSecret(uri *secrets.URI, p CreateSecretParams) (*secrets.SecretMetadata, error) {
	metadataDoc, err := s.secretMetadataDoc(uri, &p)
	if err != nil {
		return nil, errors.Trace(err)
	}
	revision := 1
	valueDoc := s.secretValueDoc(uri, revision, p.Data)
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
				C:      secretValuesC,
				Id:     valueDoc.DocID,
				Assert: txn.DocMissing,
				Insert: *valueDoc,
			},
		}
		rotateOps, err := s.secretRotationOps(uri, metadataDoc.OwnerTag, p.RotateInterval)
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
		s.updateSecretMetadataDoc(&metadataDoc, &p)
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
			valueDoc := s.secretValueDoc(uri, metadataDoc.Revision, p.Data)
			ops = append(ops, txn.Op{
				C:      secretValuesC,
				Id:     valueDoc.DocID,
				Assert: txn.DocMissing,
				Insert: *valueDoc,
			})
		}
		if p.RotateInterval != nil {
			rotateOps, err := s.secretRotationOps(uri, metadataDoc.OwnerTag, *p.RotateInterval)
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
		RotateInterval: doc.RotateInterval,
		Description:    doc.Description,
		OwnerTag:       doc.OwnerTag,
		Tags:           doc.Tags,
		Provider:       doc.Provider,
		ProviderID:     doc.ProviderID,
		Revision:       doc.Revision,
		CreateTime:     doc.CreateTime,
		UpdateTime:     doc.UpdateTime,
	}, nil
}

// GetSecretValue gets the secret value for the specified URL.
func (s *secretsStore) GetSecretValue(uri *secrets.URI, revision int) (secrets.SecretValue, error) {
	secretValuesCollection, closer := s.st.db().GetCollection(secretValuesC)
	defer closer()

	var doc secretValueDoc
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

type secretRotationDoc struct {
	DocID    string `bson:"_id"`
	TxnRevno int64  `bson:"txn-revno"`

	LastRotateTime time.Time `bson:"last-rotate-time"`

	// These fields are denormalised here so that the watcher
	// only needs to access this collection.
	RotateInterval time.Duration `bson:"rotate-interval"`
	Owner          string        `bson:"owner"`
}

func secretGlobalKey(id string) string {
	return fmt.Sprintf("secret#%s", id)
}

func secretIDFromGlobalKey(key string) string {
	id := strings.TrimPrefix(key, "secret#")
	return id
}

func (s *secretsStore) secretRotationOps(uri *secrets.URI, owner string, rotateInterval time.Duration) ([]txn.Op, error) {
	secretKey := secretGlobalKey(uri.ID)
	if rotateInterval <= 0 {
		return []txn.Op{{
			C:      secretRotateC,
			Id:     secretKey,
			Remove: true,
		}}, nil
	}
	secretRotateCollection, closer := s.st.db().GetCollection(secretRotateC)
	defer closer()

	var doc secretMetadataDoc
	err := secretRotateCollection.FindId(secretKey).One(&doc)
	if err := errors.Cause(err); err != nil && err != mgo.ErrNotFound {
		return nil, errors.Trace(err)
	}
	if err == nil {
		return []txn.Op{{
			C:      secretRotateC,
			Id:     secretKey,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"rotate-interval": rotateInterval}},
		}}, nil
	}
	return []txn.Op{{
		C:      secretRotateC,
		Id:     secretKey,
		Assert: txn.DocMissing,
		Insert: secretRotationDoc{
			DocID:          secretKey,
			RotateInterval: rotateInterval,
			Owner:          owner,
			LastRotateTime: s.st.nowToTheSecond(),
		},
	}}, nil
}

// SecretRotated records when the given secret was rotated.
func (st *State) SecretRotated(uri *secrets.URI, when time.Time) error {
	secretMetadataCollection, closer := st.db().GetCollection(secretMetadataC)
	defer closer()

	secretRotateCollection, closer2 := st.db().GetCollection(secretRotateC)
	defer closer2()

	when = when.Round(time.Second)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		var metadataDoc secretMetadataDoc
		err := secretMetadataCollection.FindId(uri.ID).One(&metadataDoc)
		if errors.Cause(err) == mgo.ErrNotFound {
			return nil, errors.NotFoundf("secret %q", uri.String())
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		secretKey := secretGlobalKey(uri.ID)

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

	iter := secretRotateCollection.Find(bson.D{{"owner", w.owner}}).Iter()
	for iter.Next(&doc) {
		uriStr := secretIDFromGlobalKey(doc.DocID)
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
			URI:            uri,
			RotateInterval: doc.RotateInterval,
			LastRotateTime: doc.LastRotateTime.UTC(),
		})
	}
	return details, errors.Trace(iter.Close())
}

func (w *secretsRotationWatcher) merge(details []corewatcher.SecretRotationChange, change watcher.Change) ([]corewatcher.SecretRotationChange, error) {
	docID := change.Id.(string)
	id := secretIDFromGlobalKey(docID)
	knownDetails, known := w.known[docID]

	doc := secretRotationDoc{}
	if change.Revno >= 0 {
		secretsRotationColl, closer := w.db.GetCollection(secretRotateC)
		defer closer()
		err := secretsRotationColl.Find(bson.D{{"_id", change.Id}, {"owner", w.owner}}).One(&doc)
		if err != nil && errors.Cause(err) != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		if err != nil {
			return details, nil
		}
	} else if known {
		for i, detail := range details {
			if detail.URI.ID == id {
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
		uriStr := secretIDFromGlobalKey(doc.DocID)
		uri, err := secrets.ParseURI(uriStr)
		if err != nil {
			return nil, errors.Annotatef(err, "invalid secret URI %q", uriStr)
		}
		w.known[docID] = rotateWatcherDetails{
			txnRevNo: doc.TxnRevno,
			URI:      uri,
		}
		details = append(details, corewatcher.SecretRotationChange{
			URI:            uri,
			RotateInterval: doc.RotateInterval,
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
