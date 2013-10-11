// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"encoding/json"
	"time"

	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/utils/set"
)

const (
	ProductMetadataPath = "streams/v1/com.ubuntu.cloud:released:imagemetadata.json"
)

// MarshalImageMetadataJSON marshals image metadata to index and products JSON.
//
// updated is the time at which the JSON file was updated.
func MarshalImageMetadataJSON(metadata []*ImageMetadata, cloudSpec *simplestreams.CloudSpec, updated time.Time) (index, products []byte, err error) {
	if index, err = MarshalImageMetadataIndexJSON(metadata, cloudSpec, updated); err != nil {
		return nil, nil, err
	}
	if products, err = MarshalImageMetadataProductsJSON(metadata, updated); err != nil {
		return nil, nil, err
	}
	return index, products, err
}

// MarshalImageMetadataIndexJSON marshals image metadata to index JSON.
//
// updated is the time at which the JSON file was updated.
func MarshalImageMetadataIndexJSON(metadata []*ImageMetadata, cloudSpec *simplestreams.CloudSpec, updated time.Time) (out []byte, err error) {
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
		"com.ubuntu.cloud:custom": &simplestreams.IndexMetadata{
			CloudName:        "custom",
			Updated:          indices.Updated,
			Format:           "products:1.0",
			DataType:         "image-ids",
			ProductsFilePath: ProductMetadataPath,
			ProductIds:       set.NewStrings(productIds...).SortedValues(),
			Clouds:           []simplestreams.CloudSpec{*cloudSpec},
		},
	}
	return json.MarshalIndent(&indices, "", "    ")
}

// MarshalImageMetadataProductsJSON marshals image metadata to products JSON.
//
// updated is the time at which the JSON file was updated.
func MarshalImageMetadataProductsJSON(metadata []*ImageMetadata, updated time.Time) (out []byte, err error) {
	var cloud simplestreams.CloudMetadata
	cloud.Updated = updated.Format(time.RFC1123Z)
	cloud.Format = "products:1.0"
	cloud.Products = make(map[string]simplestreams.MetadataCatalog)
	itemsversion := updated.Format("20060102") // YYYYMMDD
	for _, t := range metadata {
		id, err := t.productId()
		if err != nil {
			return nil, err
		}
		version, err := simplestreams.SeriesVersion(t.Release)
		if err != nil {
			return nil, err
		}
		toWrite := &ImageMetadata{
			Id: t.Id,
		}
		if catalog, ok := cloud.Products[id]; ok {
			catalog.Items[itemsversion].Items[t.Id] = t
		} else {
			catalog = simplestreams.MetadataCatalog{
				Arch:       t.Arch,
				RegionName: t.RegionName,
				Version:    version,
				Items: map[string]*simplestreams.ItemCollection{
					itemsversion: &simplestreams.ItemCollection{
						Items: map[string]interface{}{t.Id: toWrite},
					},
				},
			}
			cloud.Products[id] = catalog
		}
	}
	return json.MarshalIndent(&cloud, "", "    ")
}
