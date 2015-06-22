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

// AddMetadata implements Storage.AddMetadata.
func (s *storage) AddMetadata(metadata Metadata) error {
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
func (s *storage) FindMetadata(series, arch, stream string) (Metadata, error) {
	desiredID := createKey(series, arch, stream)
	doc, err := s.imagesMetadata(desiredID)
	if err != nil {
		return Metadata{}, errors.Trace(err)
	}
	return doc.metadata(), nil
}

// AllMetadata implements Storage.AllMetadata.
func (s *storage) AllMetadata() ([]Metadata, error) {
	var docs []imagesMetadataDoc
	if err := s.metadataCollection.Find(nil).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}
	metadata := make([]Metadata, len(docs))
	for i, doc := range docs {
		metadata[i] = doc.metadata()
	}
	return metadata, nil
}

func (c *storage) imagesMetadata(desiredID string) (imagesMetadataDoc, error) {
	var doc imagesMetadataDoc
	logger.Infof("looking for cloud image metadata with id %v", desiredID)
	err := c.metadataCollection.Find(bson.D{{"_id", desiredID}}).One(&doc)
	if err == mgo.ErrNotFound {
		return doc, errors.NotFoundf("cloud image metadata with id %v", desiredID)
	}
	return doc, err
}

type imagesMetadataDoc struct {
	Id          string `bson:"_id"`
	Series      string `bson:"series"`
	Arch        string `bson:"arch"`
	Stream      string `bson:"stream,omitempty"`
	Storage     string `bson:"root_store,omitempty"`
	VirtType    string `bson:"virt,omitempty"`
	RegionAlias string `bson:"crsn,omitempty"`
	RegionName  string `bson:"region,omitempty"`
	Endpoint    string `bson:"endpoint,omitempty"`
}

func (m imagesMetadataDoc) metadata() Metadata {
	return Metadata{
		Series:      m.Series,
		Arch:        m.Arch,
		Stream:      m.Stream,
		Storage:     m.Storage,
		VirtType:    m.VirtType,
		RegionAlias: m.RegionAlias,
		RegionName:  m.RegionName,
		Endpoint:    m.Endpoint,
	}
}

func (m imagesMetadataDoc) updates() bson.D {
	return bson.D{
		{"series", m.Series},
		{"arch", m.Arch},
		{"stream", m.Stream},
		{"root_store", m.Storage},
		{"virt", m.VirtType},
		{"crsn", m.RegionAlias},
		{"region", m.RegionName},
		{"endpoint", m.Endpoint},
	}
}

func (m *Metadata) mongoDoc() imagesMetadataDoc {
	return imagesMetadataDoc{
		Id:          m.key(),
		Series:      m.Series,
		Arch:        m.Arch,
		Stream:      m.Stream,
		Storage:     m.Storage,
		VirtType:    m.VirtType,
		RegionAlias: m.RegionAlias,
		RegionName:  m.RegionName,
		Endpoint:    m.Endpoint,
	}
}

func streamKey(stream string) string {
	if stream != "" {
		return stream
	}
	// Since stream is optional,when omitted, assume "released" is desired.
	return imagemetadata.ReleasedStream
}

var createKey = func(series, arch, stream string) string {
	return fmt.Sprintf("%s-%s-%s", series, arch, streamKey(stream))
}

func (im *Metadata) key() string {
	return createKey(im.Series, im.Arch, im.Stream)
}
