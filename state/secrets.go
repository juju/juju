// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"
	jujutxn "github.com/juju/txn/v2"
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
	Type           secrets.SecretType
	Owner          string
	Path           string
	RotateInterval time.Duration
	Description    string
	Status         secrets.SecretStatus
	Tags           map[string]string
	Params         map[string]interface{}
	Data           secrets.SecretData
}

// UpdateSecretParams are used to update a secret.
type UpdateSecretParams struct {
	RotateInterval *time.Duration
	Description    *string
	Status         *secrets.SecretStatus
	Tags           *map[string]string
	Params         map[string]interface{}
	Data           secrets.SecretData
}

func (u *UpdateSecretParams) hasUpdate() bool {
	return u.RotateInterval != nil ||
		u.Description != nil ||
		u.Status != nil ||
		u.Tags != nil ||
		len(u.Data) > 0 ||
		len(u.Params) > 0
}

// TODO(wallyworld)
type SecretsFilter struct{}

// SecretsStore instances use mongo as a secrets store.
type SecretsStore interface {
	CreateSecret(URL *secrets.URL, p CreateSecretParams) (*secrets.SecretMetadata, error)
	UpdateSecret(URL *secrets.URL, p UpdateSecretParams) (*secrets.SecretMetadata, error)
	GetSecret(URL *secrets.URL) (*secrets.SecretMetadata, error)
	GetSecretValue(URL *secrets.URL) (secrets.SecretValue, error)
	ListSecrets(filter SecretsFilter) ([]*secrets.SecretMetadata, error)
}

// NewSecretsStore creates a new mongo backed secrets store.
func NewSecretsStore(st *State) *secretsStore {
	return &secretsStore{st: st}
}

type secretMetadataDoc struct {
	DocID string `bson:"_id"`

	Path           string            `bson:"path"`
	Version        int               `bson:"version"`
	Owner          string            `bson:"owner"`
	RotateInterval time.Duration     `bson:"rotate-interval"`
	Description    string            `bson:"description"`
	Status         string            `bson:"status"`
	Tags           map[string]string `bson:"tags"`
	ID             int               `bson:"id"`
	Provider       string            `bson:"provider"`
	ProviderID     string            `bson:"provider-id"`
	Revision       int               `bson:"revision"`
	CreateTime     time.Time         `bson:"create-time"`
	UpdateTime     time.Time         `bson:"update-time"`
}

type secretValueDoc struct {
	DocID string `bson:"_id"`

	Data secretsDataMap `bson:"data"`
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

func (s *secretsStore) secretMetadataDoc(baseURL *secrets.URL, p *CreateSecretParams) (*secrets.URL, *secretMetadataDoc, error) {
	id, err := sequenceWithMin(s.st, "secret", 1)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	interval := p.RotateInterval
	if interval < 0 {
		interval = 0
	}
	md := &secretMetadataDoc{
		DocID:          baseURL.String(),
		Path:           p.Path,
		Version:        p.Version,
		Owner:          p.Owner,
		RotateInterval: interval,
		Status:         string(p.Status),
		Description:    p.Description,
		Tags:           p.Tags,
		ID:             id,
		Provider:       p.ProviderLabel,
		ProviderID:     "",
		Revision:       1,
		CreateTime:     s.st.nowToTheSecond(),
		UpdateTime:     s.st.nowToTheSecond(),
	}
	return baseURL.WithRevision(md.Revision), md, nil
}

func (s *secretsStore) updateSecretMetadataDoc(doc *secretMetadataDoc, baseURL *secrets.URL, p *UpdateSecretParams) *secrets.URL {
	if p.RotateInterval != nil {
		doc.RotateInterval = *p.RotateInterval
	}
	if p.Description != nil {
		doc.Description = *p.Description
	}
	if p.Status != nil {
		doc.Status = string(*p.Status)
	}
	if p.Tags != nil {
		doc.Tags = *p.Tags
	}
	if len(p.Data) > 0 {
		doc.Revision++
	}
	updatedURL := *baseURL.WithRevision(doc.Revision)
	doc.UpdateTime = s.st.nowToTheSecond()
	return &updatedURL
}

func (s *secretsStore) secretValueDoc(url *secrets.URL, data secrets.SecretData) *secretValueDoc {
	dataCopy := make(secretsDataMap)
	for k, v := range data {
		dataCopy[k] = v
	}
	return &secretValueDoc{
		DocID: url.String(),
		Data:  dataCopy,
	}
}

// CreateSecret creates a new secret.
func (s *secretsStore) CreateSecret(baseURL *secrets.URL, p CreateSecretParams) (*secrets.SecretMetadata, error) {
	URL, metadataDoc, err := s.secretMetadataDoc(baseURL, &p)
	if err != nil {
		return nil, errors.Trace(err)
	}
	valueDoc := s.secretValueDoc(URL, p.Data)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if _, err := s.GetSecretValue(URL); err == nil {
				return nil, errors.AlreadyExistsf("secret value for %q", URL.ID())
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
		rotateOps, err := s.secretRotationOps(metadataDoc.ID, URL, metadataDoc.Owner, p.RotateInterval)
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
func (s *secretsStore) UpdateSecret(url *secrets.URL, p UpdateSecretParams) (*secrets.SecretMetadata, error) {
	if url.Revision > 0 {
		return nil, errors.New("cannot specify a revision when updating a secret")
	}
	if !p.hasUpdate() {
		return nil, errors.New("must specify a new value or metadata to update a secret")
	}
	secretMetadataCollection, closer := s.st.db().GetCollection(secretMetadataC)
	defer closer()

	var metadataDoc secretMetadataDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		err := secretMetadataCollection.FindId(url.WithRevision(0).ID()).One(&metadataDoc)
		if errors.Cause(err) == mgo.ErrNotFound {
			return nil, errors.NotFoundf("secret %q", url.ID())
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		updatedURL := s.updateSecretMetadataDoc(&metadataDoc, url, &p)
		ops := []txn.Op{
			{
				C:      secretMetadataC,
				Id:     metadataDoc.DocID,
				Assert: txn.DocExists,
				Update: bson.M{"$set": metadataDoc},
			},
		}
		if len(p.Data) > 0 {
			if _, err := s.GetSecretValue(updatedURL); err == nil {
				return nil, errors.AlreadyExistsf("secret value for %q", url.ID())
			}
			valueDoc := s.secretValueDoc(updatedURL, p.Data)
			ops = append(ops, txn.Op{
				C:      secretValuesC,
				Id:     valueDoc.DocID,
				Assert: txn.DocMissing,
				Insert: *valueDoc,
			})
		}
		if p.RotateInterval != nil {
			rotateOps, err := s.secretRotationOps(metadataDoc.ID, url, metadataDoc.Owner, *p.RotateInterval)
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
	url, err := secrets.ParseURL(doc.DocID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &secrets.SecretMetadata{
		URL:            url,
		Path:           doc.Path,
		Version:        doc.Version,
		RotateInterval: doc.RotateInterval,
		Status:         secrets.SecretStatus(doc.Status),
		Description:    doc.Description,
		Tags:           doc.Tags,
		ID:             doc.ID,
		Provider:       doc.Provider,
		ProviderID:     doc.ProviderID,
		Revision:       doc.Revision,
		CreateTime:     doc.CreateTime,
		UpdateTime:     doc.UpdateTime,
	}, nil
}

// GetSecretValue gets the secret value for the specified URL.
func (s *secretsStore) GetSecretValue(URL *secrets.URL) (secrets.SecretValue, error) {
	secretValuesCollection, closer := s.st.db().GetCollection(secretValuesC)
	defer closer()

	var doc secretValueDoc
	err := secretValuesCollection.FindId(URL.ID()).One(&doc)
	if errors.Cause(err) == mgo.ErrNotFound {
		return nil, errors.NotFoundf("secret %q", URL.ID())
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	data := make(secrets.SecretData)
	for k, v := range doc.Data {
		if URL.Attribute != "" && k != URL.Attribute {
			continue
		}
		data[k] = fmt.Sprintf("%v", v)
	}
	if URL.Attribute != "" && len(data) == 0 {
		return nil, errors.NotFoundf("secret attribute %q", URL.Attribute)
	}
	return secrets.NewSecretValue(data), nil
}

// GetSecret gets the secret metadata for the specified URL.
func (s *secretsStore) GetSecret(URL *secrets.URL) (*secrets.SecretMetadata, error) {
	secretMetadataCollection, closer := s.st.db().GetCollection(secretMetadataC)
	defer closer()

	var doc secretMetadataDoc
	err := secretMetadataCollection.FindId(URL.WithRevision(0).ID()).One(&doc)
	if errors.Cause(err) == mgo.ErrNotFound {
		return nil, errors.NotFoundf("secret %q", URL.ID())
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
	URL            string        `bson:"url"`
	RotateInterval time.Duration `bson:"rotate-interval"`
	Owner          string        `bson:"owner"`
}

func secretGlobalKey(secretID int) string {
	return fmt.Sprintf("secret#%d", secretID)
}

func secretIDFromGlobalKey(key string) (int, error) {
	id := strings.TrimLeft(key, "secret#")
	return strconv.Atoi(id)
}

func (s *secretsStore) secretRotationOps(id int, URL *secrets.URL, owner string, rotateInterval time.Duration) ([]txn.Op, error) {
	secretKey := secretGlobalKey(id)
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
			URL:            URL.WithRevision(0).ID(),
			RotateInterval: rotateInterval,
			Owner:          owner,
			LastRotateTime: s.st.nowToTheSecond(),
		},
	}}, nil
}

// SecretRotated records when the given secret was rotated.
func (st *State) SecretRotated(url *secrets.URL, when time.Time) error {
	if url.Revision > 0 {
		return errors.New("cannot specify a revision when updating a secret rotation time")
	}
	secretMetadataCollection, closer := st.db().GetCollection(secretMetadataC)
	defer closer()

	secretRotateCollection, closer2 := st.db().GetCollection(secretRotateC)
	defer closer2()

	when = when.Round(time.Second)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		var metadataDoc secretMetadataDoc
		err := secretMetadataCollection.FindId(url.WithRevision(0).ID()).One(&metadataDoc)
		if errors.Cause(err) == mgo.ErrNotFound {
			return nil, errors.NotFoundf("secret %q", url.ID())
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		secretKey := secretGlobalKey(metadataDoc.ID)

		var currentDoc secretRotationDoc
		err = secretRotateCollection.FindId(secretKey).One(&currentDoc)
		if errors.Cause(err) == mgo.ErrNotFound {
			return nil, errors.NotFoundf("rotation info for secret %q", url.ID())
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
	URL      *secrets.URL
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
		id, err := secretIDFromGlobalKey(doc.DocID)
		if err != nil {
			_ = iter.Close()
			return nil, errors.Annotatef(err, "invalid secret key %q", doc.DocID)
		}
		url, err := secrets.ParseURL(doc.URL)
		if err != nil {
			_ = iter.Close()
			return nil, errors.Annotatef(err, "invalid secret URL %q", doc.URL)
		}
		w.known[doc.DocID] = rotateWatcherDetails{
			txnRevNo: doc.TxnRevno,
			URL:      url,
		}
		details = append(details, corewatcher.SecretRotationChange{
			ID:             id,
			URL:            url,
			RotateInterval: doc.RotateInterval,
			LastRotateTime: doc.LastRotateTime.UTC(),
		})
	}
	return details, errors.Trace(iter.Close())
}

func (w *secretsRotationWatcher) merge(details []corewatcher.SecretRotationChange, change watcher.Change) ([]corewatcher.SecretRotationChange, error) {
	docID := change.Id.(string)
	id, err := secretIDFromGlobalKey(docID)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid secret key %q", change.Id)
	}
	knownDetails, known := w.known[docID]

	doc := secretRotationDoc{}
	if change.Revno >= 0 {
		secretsRotationColl, closer := w.db.GetCollection(secretRotateC)
		defer closer()
		err = secretsRotationColl.Find(bson.D{{"_id", change.Id}, {"owner", w.owner}}).One(&doc)
		if err != nil && errors.Cause(err) != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		if err != nil {
			return details, nil
		}
	} else if known {
		for i, detail := range details {
			if detail.ID == id {
				details[i].RotateInterval = 0
				return details, nil
			}
		}
		details = append(details, corewatcher.SecretRotationChange{
			ID:             id,
			URL:            knownDetails.URL,
			RotateInterval: 0,
		})
		return details, nil
	}
	if doc.TxnRevno > knownDetails.txnRevNo {
		url, err := secrets.ParseURL(doc.URL)
		if err != nil {
			return nil, errors.Annotatef(err, "invalid secret URL %q", doc.URL)
		}
		w.known[docID] = rotateWatcherDetails{
			txnRevNo: doc.TxnRevno,
			URL:      url,
		}
		details = append(details, corewatcher.SecretRotationChange{
			ID:             id,
			URL:            url,
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
