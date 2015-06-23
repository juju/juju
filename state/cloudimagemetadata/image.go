// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudimagemetadata

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/environs/imagemetadata"
)

var logger = loggo.GetLogger("juju.state.cloudimagemetadata")

type storage struct {
	envUUID            string
	metadataCollection *mgo.Collection
	txnRunner          jujutxn.Runner
}

var _ Storage = (*storage)(nil)

// NewStorage constructs a new Storage that stores  image metadata
// in the provided collection using the provided transaction runner.
func NewStorage(
	envUUID string,
	metadataCollection *mgo.Collection,
	runner jujutxn.Runner,
) Storage {
	return &storage{
		envUUID:            envUUID,
		metadataCollection: metadataCollection,
		txnRunner:          runner,
	}
}

// SaveMetadata implements Storage.SaveMetadata.
func (s *storage) SaveMetadata(metadata Metadata) error {
	newDoc := metadata.mongoDoc()
	buildTxn := func(attempt int) ([]txn.Op, error) {
		op := txn.Op{
			C:  s.metadataCollection.Name,
			Id: newDoc.Id,
		}
		if attempt == 0 {
			// On the first attempt we assume we're adding new cloud image metadata.
			op.Assert = txn.DocMissing
			op.Insert = &newDoc
			logger.Debugf("inserting cloud image metadata for %v", newDoc.Id)
		} else {
			// Subsequent attempts to add metadata will update the fields.
			op.Assert = txn.DocExists
			op.Update = bson.D{{"$set", newDoc.updates()}}
			logger.Debugf("updating cloud image metadata for %v", newDoc.Id)
		}
		return []txn.Op{op}, nil
	}

	err := s.txnRunner.Run(buildTxn)
	if err != nil {
		return errors.Annotatef(err, "cannot add cloud image metadata for %v", newDoc.Id)
	}
	return nil
}

// FindMetadata implements Storage.FindMetadata.
func (s *storage) FindMetadata(criteria Metadata) ([]Metadata, error) {
	return s.imagesMetadata(criteria)
}

// AllMetadata implements Storage.AllMetadata.
func (s *storage) AllMetadata() ([]Metadata, error) {
	return s.imagesMetadata(Metadata{})
}

func (s *storage) imagesMetadata(criteria Metadata) ([]Metadata, error) {
	searchCriteria := searchClauses(criteria)
	var docs []imagesMetadataDoc
	if err := s.metadataCollection.Find(searchCriteria).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}
	if searchCriteria != nil && len(docs) == 0 {
		// If criteria had values and no metadata was found, err
		return nil, errors.NotFoundf("cloud image metadata")
	}
	metadata := make([]Metadata, len(docs))
	for i, doc := range docs {
		metadata[i] = doc.metadata()
	}
	return metadata, nil
}

var searchClauses = func(criteria Metadata) bson.D {
	all := bson.D{}

	if criteria.Stream != "" {
		all = append(all, bson.DocElem{"stream", criteria.Stream})
	}

	if criteria.Region != "" {
		all = append(all, bson.DocElem{"region", criteria.Region})
	}

	if criteria.Series != "" {
		all = append(all, bson.DocElem{"series", criteria.Series})
	}

	if criteria.Arch != "" {
		all = append(all, bson.DocElem{"arch", criteria.Arch})
	}

	if criteria.VirtualType != "" {
		all = append(all, bson.DocElem{"virtual_type", criteria.VirtualType})
	}

	if criteria.RootStorageType != "" {
		all = append(all, bson.DocElem{"root_storage_type", criteria.RootStorageType})
	}

	if len(all.Map()) == 0 {
		return nil
	}
	return all
}

type imagesMetadataDoc struct {
	Id              string `bson:"_id"`
	Stream          string `bson:"stream,omitempty"`
	Region          string `bson:"region,omitempty"`
	Series          string `bson:"series"`
	Arch            string `bson:"arch"`
	VirtualType     string `bson:"virtual_type,omitempty"`
	RootStorageType string `bson:"root_storage_type,omitempty"`
}

func (m imagesMetadataDoc) metadata() Metadata {
	return Metadata{
		Series:          m.Series,
		Arch:            m.Arch,
		Stream:          m.Stream,
		RootStorageType: m.RootStorageType,
		VirtualType:     m.VirtualType,
		Region:          m.Region,
	}
}

func (m imagesMetadataDoc) updates() bson.D {
	return bson.D{
		{"stream", m.Stream},
		{"region", m.Region},
		{"series", m.Series},
		{"arch", m.Arch},
		{"virtual_type", m.VirtualType},
		{"root_storage_type", m.RootStorageType},
	}
}

func (m *Metadata) mongoDoc() imagesMetadataDoc {
	return imagesMetadataDoc{
		Id:              key(m),
		Stream:          m.Stream,
		Region:          m.Region,
		Series:          m.Series,
		Arch:            m.Arch,
		VirtualType:     m.VirtualType,
		RootStorageType: m.RootStorageType,
	}
}

func streamKey(stream string) string {
	if stream != "" {
		return stream
	}
	// Since stream is optional,when omitted, assume "released" is desired.
	return imagemetadata.ReleasedStream
}

func key(m *Metadata) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s:%s",
		streamKey(m.Stream),
		m.Region,
		m.Series,
		m.Arch,
		m.VirtualType,
		m.RootStorageType)
}
