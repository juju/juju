// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filestorage

import (
	"github.com/juju/errors"
)

// Convert turns a Document into a Metadata if possible.
func Convert(doc Document) (Metadata, error) {
	meta, ok := doc.(Metadata)
	if !ok {
		return nil, errors.Errorf("expected a Metadata doc, got %v", doc)
	}
	return meta, nil
}

// MetadataDocStorage provides the MetadataStorage methods than can be
// derived from DocStorage methods.  To fully implement MetadataStorage,
// this type must be embedded in a type that implements the remaining
// methods.
type MetadataDocStorage struct {
	DocStorage
}

// Metadata implements MetadataStorage.Metadata.
func (s *MetadataDocStorage) Metadata(id string) (Metadata, error) {
	doc, err := s.Doc(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	meta, err := Convert(doc)
	return meta, errors.Trace(err)
}

// ListMetadata implements MetadataStorage.ListMetadata.
func (s *MetadataDocStorage) ListMetadata() ([]Metadata, error) {
	docs, err := s.ListDocs()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var metaList []Metadata
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		meta, err := Convert(doc)
		if err != nil {
			return nil, errors.Trace(err)
		}
		metaList = append(metaList, meta)
	}
	return metaList, nil
}

// ListMetadata implements MetadataStorage.ListMetadata.
func (s *MetadataDocStorage) AddMetadata(meta Metadata) (string, error) {
	id, err := s.AddDoc(meta)
	return id, errors.Trace(err)
}

// ListMetadata implements MetadataStorage.ListMetadata.
func (s *MetadataDocStorage) RemoveMetadata(id string) error {
	return errors.Trace(s.RemoveDoc(id))
}
