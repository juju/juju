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
	ImageContentId      = "com.ubuntu.cloud:custom"
)

// MarshalImageMetadataJSON marshals image metadata to index and products JSON.
//
// updated is the time at which the JSON file was updated.
func MarshalImageMetadataJSON(metadata []*ImageMetadata, cloudSpec []simplestreams.CloudSpec,
	updated time.Time) (index, products []byte, err error) {

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
func MarshalImageMetadataIndexJSON(metadata []*ImageMetadata, cloudSpec []simplestreams.CloudSpec,
	updated time.Time) (out []byte, err error) {

	productIds := make([]string, len(metadata))
	for i, t := range metadata {
		productIds[i] = t.productId()
	}
	var indices simplestreams.Indices
	indices.Updated = updated.Format(time.RFC1123Z)
	indices.Format = "index:1.0"
	indices.Indexes = map[string]*simplestreams.IndexMetadata{
		ImageContentId: &simplestreams.IndexMetadata{
			CloudName:        "custom",
			Updated:          indices.Updated,
			Format:           "products:1.0",
			DataType:         "image-ids",
			ProductsFilePath: ProductMetadataPath,
			ProductIds:       set.NewStrings(productIds...).SortedValues(),
			Clouds:           cloudSpec,
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
	cloud.ContentId = ImageContentId
	cloud.Products = make(map[string]simplestreams.MetadataCatalog)
	itemsversion := updated.Format("20060201") // YYYYMMDD
	for _, t := range metadata {
		toWrite := &ImageMetadata{
			Id:         t.Id,
			RegionName: t.RegionName,
			Endpoint:   t.Endpoint,
			VirtType:   t.VirtType,
		}
		if catalog, ok := cloud.Products[t.productId()]; ok {
			catalog.Items[itemsversion].Items[t.Id] = toWrite
		} else {
			catalog = simplestreams.MetadataCatalog{
				Arch:    t.Arch,
				Version: t.Version,
				Items: map[string]*simplestreams.ItemCollection{
					itemsversion: &simplestreams.ItemCollection{
						Items: map[string]interface{}{t.Id: toWrite},
					},
				},
			}
			cloud.Products[t.productId()] = catalog
		}
	}
	return json.MarshalIndent(&cloud, "", "    ")
}
