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

	"github.com/juju/utils/set"

	"github.com/juju/juju/environs/simplestreams"
)

// ToolsContentId returns the tools content id for the given stream.
func ToolsContentId(stream string) string {
	return fmt.Sprintf("com.ubuntu.juju:%s:tools", stream)
}

// ProductMetadataPath returns the tools product metadata path for the given stream.
func ProductMetadataPath(stream string) string {
	return fmt.Sprintf("streams/v1/com.ubuntu.juju-%s-tools.json", stream)
}

// MarshalToolsMetadataJSON marshals tools metadata to index and products JSON.
// updated is the time at which the JSON file was updated.
func MarshalToolsMetadataJSON(metadata map[string][]*ToolsMetadata, updated time.Time) (index []byte, products map[string][]byte, err error) {
	if index, err = MarshalToolsMetadataIndexJSON(metadata, updated); err != nil {
		return nil, nil, err
	}
	if products, err = MarshalToolsMetadataProductsJSON(metadata, updated); err != nil {
		return nil, nil, err
	}
	return index, products, err
}

// MarshalToolsMetadataIndexJSON marshals tools metadata to index JSON.
// updated is the time at which the JSON file was updated.
func MarshalToolsMetadataIndexJSON(streamMetadata map[string][]*ToolsMetadata, updated time.Time) (out []byte, err error) {
	var indices simplestreams.Indices
	indices.Updated = updated.Format(time.RFC1123Z)
	indices.Format = simplestreams.IndexFormat
	indices.Indexes = make(map[string]*simplestreams.IndexMetadata, len(streamMetadata))
	for stream, metadata := range streamMetadata {
		productIds := make([]string, len(metadata))
		for i, t := range metadata {
			productIds[i], err = t.productId()
			if err != nil {
				return nil, err
			}
		}
		indexMetadata := &simplestreams.IndexMetadata{
			Updated:          indices.Updated,
			Format:           simplestreams.ProductFormat,
			DataType:         ContentDownload,
			ProductsFilePath: ProductMetadataPath(stream),
			ProductIds:       set.NewStrings(productIds...).SortedValues(),
		}
		indices.Indexes[ToolsContentId(stream)] = indexMetadata
	}
	return json.MarshalIndent(&indices, "", "    ")
}

// MarshalToolsMetadataProductsJSON marshals tools metadata to products JSON.
// updated is the time at which the JSON file was updated.
func MarshalToolsMetadataProductsJSON(
	streamMetadata map[string][]*ToolsMetadata, updated time.Time,
) (out map[string][]byte, err error) {

	out = make(map[string][]byte, len(streamMetadata))
	for stream, metadata := range streamMetadata {
		var cloud simplestreams.CloudMetadata
		cloud.Updated = updated.Format(time.RFC1123Z)
		cloud.Format = simplestreams.ProductFormat
		cloud.ContentId = ToolsContentId(stream)
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
		if out[stream], err = json.MarshalIndent(&cloud, "", "    "); err != nil {
			return nil, err
		}
	}
	return out, nil
}
