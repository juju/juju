// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/mongo/utils"
)

// CreateSecretParams are used to create a secret.
// TODO(wallyworld) - add tags and description etc
type CreateSecretParams struct {
	ControllerUUID string
	ModelUUID      string
	ProviderLabel  string
	Version        int
	Type           string
	Path           string
	RotateInterval time.Duration
	Params         map[string]interface{}
	Data           secrets.SecretData
}

// UpdateSecretParams are used to update a secret.
// TODO(wallyworld) - add tags and description etc
type UpdateSecretParams struct {
	RotateInterval time.Duration
	Params         map[string]interface{}
	Data           secrets.SecretData
}

// TODO(wallyworld)
type SecretsFilter struct{}

// SecretsStore instances use mongo as a secrets store.
type SecretsStore interface {
	CreateSecret(p CreateSecretParams) (*secrets.SecretMetadata, error)
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
	RotateInterval time.Duration     `bson:"rotate-interval"`
	Description    string            `bson:"description"`
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

func (s *secretsStore) secretMetadataDoc(p *CreateSecretParams) (*secrets.URL, *secretMetadataDoc, error) {
	id, err := sequenceWithMin(s.st, "secret", 1)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	interval := p.RotateInterval
	if interval < 0 {
		interval = 0
	}
	URL := secrets.NewSimpleURL(p.Version, p.Path)
	URL.ControllerUUID = p.ControllerUUID
	URL.ModelUUID = p.ModelUUID
	md := &secretMetadataDoc{
		DocID:          URL.String(),
		Path:           p.Path,
		Version:        p.Version,
		RotateInterval: interval,
		Description:    "",
		Tags:           nil,
		ID:             id,
		Provider:       p.ProviderLabel,
		ProviderID:     "",
		Revision:       1,
		CreateTime:     s.st.nowToTheSecond(),
		UpdateTime:     s.st.nowToTheSecond(),
	}
	return URL.WithRevision(md.Revision), md, nil
}

func (s *secretsStore) updateSecretMetadataDoc(doc *secretMetadataDoc, baseURL *secrets.URL, p *UpdateSecretParams) *secrets.URL {
	if p.RotateInterval >= 0 {
		doc.RotateInterval = p.RotateInterval
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
func (s *secretsStore) CreateSecret(p CreateSecretParams) (*secrets.SecretMetadata, error) {
	URL, metadataDoc, err := s.secretMetadataDoc(&p)
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
		return ops, nil
	}
	err = s.st.db().Run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.toSecretMetadata(metadataDoc)
}

// UpdateSecret updates an existing secret.
func (s *secretsStore) UpdateSecret(URL *secrets.URL, p UpdateSecretParams) (*secrets.SecretMetadata, error) {
	if URL.Revision > 0 {
		return nil, errors.New("cannot specify a revision when updating a secret")
	}
	if len(p.Data) == 0 && p.RotateInterval < 0 && len(p.Params) == 0 {
		return nil, errors.New("must specify a new value or metadata to update a secret")
	}
	secretMetadataCollection, closer := s.st.db().GetCollection(secretMetadataC)
	defer closer()

	var metadataDoc secretMetadataDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		err := secretMetadataCollection.FindId(URL.ID()).One(&metadataDoc)
		if errors.Cause(err) == mgo.ErrNotFound {
			return nil, errors.NotFoundf("secret %q", URL.ID())
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		updatedURL := s.updateSecretMetadataDoc(&metadataDoc, URL, &p)
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
				return nil, errors.AlreadyExistsf("secret value for %q", URL.ID())
			}
			valueDoc := s.secretValueDoc(updatedURL, p.Data)
			ops = append(ops, txn.Op{
				C:      secretValuesC,
				Id:     valueDoc.DocID,
				Assert: txn.DocMissing,
				Insert: *valueDoc,
			})
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
	URL, err := secrets.ParseURL(doc.DocID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &secrets.SecretMetadata{
		URL:            URL,
		Path:           doc.Path,
		Version:        doc.Version,
		RotateInterval: doc.RotateInterval,
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
		return nil, errors.NotFoundf("secret %q", URL.String())
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
		return nil, errors.NotFoundf("secret %q", URL.String())
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
