// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The tools package supports locating, parsing, and filtering Ubuntu tools metadata in simplestreams format.
// See http://launchpad.net/simplestreams and in particular the doc/README file in that project for more information
// about the file formats.
package tools

import (
	"encoding/json"
	"fmt"
	"time"

	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/utils/set"
)

const (
	ProductMetadataPath = "streams/v1/com.ubuntu.juju:released:tools.json"
	ToolsContentId      = "com.ubuntu.juju:released:tools"
)

// MarshalToolsMetadataJSON marshals tools metadata to index and products JSON.
//
// updated is the time at which the JSON file was updated.
func MarshalToolsMetadataJSON(metadata []*ToolsMetadata, updated time.Time) (index, products []byte, err error) {
	if index, err = MarshalToolsMetadataIndexJSON(metadata, updated); err != nil {
		return nil, nil, err
	}
	if products, err = MarshalToolsMetadataProductsJSON(metadata, updated); err != nil {
		return nil, nil, err
	}
	return index, products, err
}

// MarshalToolsMetadataIndexJSON marshals tools metadata to index JSON.
//
// updated is the time at which the JSON file was updated.
func MarshalToolsMetadataIndexJSON(metadata []*ToolsMetadata, updated time.Time) (out []byte, err error) {
	productIds := make([]string, len(metadata))
	for i, t := range metadata {
		productIds[i], err = t.productId()
		if err != nil {
			return nil, err
		}
	}
	var indices simplestreams.Indices
	indices.Updated = updated.Format(time.RFC1123Z)
	indices.Format = "index:1.0"
	indices.Indexes = map[string]*simplestreams.IndexMetadata{
		ToolsContentId: &simplestreams.IndexMetadata{
			Updated:          indices.Updated,
			Format:           "products:1.0",
			DataType:         "content-download",
			ProductsFilePath: ProductMetadataPath,
			ProductIds:       set.NewStrings(productIds...).SortedValues(),
		},
	}
	return json.MarshalIndent(&indices, "", "    ")
}

// MarshalToolsMetadataProductsJSON marshals tools metadata to products JSON.
//
// updated is the time at which the JSON file was updated.
func MarshalToolsMetadataProductsJSON(metadata []*ToolsMetadata, updated time.Time) (out []byte, err error) {
	var cloud simplestreams.CloudMetadata
	cloud.Updated = updated.Format(time.RFC1123Z)
	cloud.Format = "products:1.0"
	cloud.ContentId = ToolsContentId
	cloud.Products = make(map[string]simplestreams.MetadataCatalog)
	itemsversion := updated.Format("20060102") // YYYYMMDD
	for _, t := range metadata {
		id, err := t.productId()
		if err != nil {
			return nil, err
		}
		itemid := fmt.Sprintf("%s-%s-%s", t.Version, t.Release, t.Arch)
		if catalog, ok := cloud.Products[id]; ok {
			catalog.Items[itemsversion].Items[itemid] = t
		} else {
			catalog = simplestreams.MetadataCatalog{
				Arch:    t.Arch,
				Version: t.Version,
				Items: map[string]*simplestreams.ItemCollection{
					itemsversion: &simplestreams.ItemCollection{
						Items: map[string]interface{}{itemid: t},
					},
				},
			}
			cloud.Products[id] = catalog
		}
	}
	return json.MarshalIndent(&cloud, "", "    ")
}
