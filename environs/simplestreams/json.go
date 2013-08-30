// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

import (
	"encoding/json"
	"reflect"
)

// The following structs define the data model used in the JSON metadata files.
// Not every model attribute is defined here, only the ones we care about.
// See the doc/README file in lp:simplestreams for more information.

// Metadata attribute values may point to a map of attribute values (aka aliases) and these attributes
// are used to override/augment the existing attributes.
type attributeValues map[string]string
type aliasesByAttribute map[string]attributeValues

type CloudMetadata struct {
	Products map[string]MetadataCatalog    `json:"products"`
	Aliases  map[string]aliasesByAttribute `json:"_aliases,omitempty"`
	Updated  string                        `json:"updated"`
	Format   string                        `json:"format"`
}

type MetadataCatalog struct {
	Series     string `json:"release,omitempty"`
	Version    string `json:"version,omitempty"`
	Arch       string `json:"arch,omitempty"`
	RegionName string `json:"region,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`

	// Items is a mapping from version to an ItemCollection,
	// where the version is the date the items were produced,
	// in the format YYYYMMDD.
	Items map[string]*ItemCollection `json:"versions"`
}

type ItemCollection struct {
	rawItems   map[string]*json.RawMessage
	Items      map[string]interface{} `json:"items"`
	Arch       string                 `json:"arch,omitempty"`
	Version    string                 `json:"version,omitempty"`
	RegionName string                 `json:"region,omitempty"`
	Endpoint   string                 `json:"endpoint,omitempty"`
}

// itemCollection is a clone of ItemCollection, but
// does not implement json.Unmarshaler.
type itemCollection struct {
	Items      map[string]*json.RawMessage `json:"items"`
	Arch       string                      `json:"arch,omitempty"`
	Version    string                      `json:"version,omitempty"`
	RegionName string                      `json:"region,omitempty"`
	Endpoint   string                      `json:"endpoint,omitempty"`
}

// ItemsCollection.UnmarshalJSON unmarshals the ItemCollection,
// storing the raw bytes for each item. These can later be
// unmarshalled again into product-specific types.
func (c *ItemCollection) UnmarshalJSON(b []byte) error {
	var raw itemCollection
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	c.rawItems = raw.Items
	c.Items = make(map[string]interface{}, len(raw.Items))
	c.Arch = raw.Arch
	c.Version = raw.Version
	c.RegionName = raw.RegionName
	c.Endpoint = raw.Endpoint
	for key, rawv := range raw.Items {
		var v interface{}
		if err := json.Unmarshal([]byte(*rawv), &v); err != nil {
			return err
		}
		c.Items[key] = v
	}
	return nil
}

func (c *ItemCollection) construct(itemType reflect.Type) error {
	for key, rawv := range c.rawItems {
		itemValuePtr := reflect.New(itemType)
		err := json.Unmarshal([]byte(*rawv), itemValuePtr.Interface())
		if err != nil {
			return err
		}
		c.Items[key] = itemValuePtr.Interface()
	}
	return nil
}

// These structs define the model used for metadata indices.

type Indices struct {
	Indexes map[string]*IndexMetadata `json:"index"`
	Updated string                    `json:"updated"`
	Format  string                    `json:"format"`
}

// Exported for testing.
type IndexReference struct {
	Indices
	BaseURL     string
	valueParams ValueParams
}

type IndexMetadata struct {
	Updated          string      `json:"updated"`
	Format           string      `json:"format"`
	DataType         string      `json:"datatype"`
	CloudName        string      `json:"cloudname,omitempty"`
	Clouds           []CloudSpec `json:"clouds,omitempty"`
	ProductsFilePath string      `json:"path"`
	ProductIds       []string    `json:"products"`
}

// These structs define the model used to describe download mirrors.

type MirrorRefs struct {
	Mirrors map[string][]MirrorReference `json:"mirrors"`
	Updated string                       `json:"updated"`
	Format  string                       `json:"format"`
}

type MirrorReference struct {
	Updated  string      `json:"updated"`
	Format   string      `json:"format"`
	DataType string      `json:"datatype"`
	Path     string      `json:"path"`
	Clouds   []CloudSpec `json:"clouds"`
}

type MirrorMetadata struct {
	Updated string                  `json:"updated"`
	Format  string                  `json:"format"`
	Mirrors map[string][]MirrorInfo `json:"mirrors"`
}

type MirrorInfo struct {
	Clouds    []CloudSpec `json:"clouds"`
	MirrorURL string      `json:"mirror"`
	Path      string      `json:"path"`
}
