// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"encoding/json"
	"time"

	"github.com/juju/utils/set"

	"github.com/juju/juju/environs/simplestreams"
)

const (
	ProductMetadataPath = "streams/v1/com.ubuntu.cloud-released-imagemetadata.json"
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
	indices.Format = simplestreams.IndexFormat
	indices.Indexes = map[string]*simplestreams.IndexMetadata{
		ImageContentId: {
			CloudName:        "custom",
			Updated:          indices.Updated,
			Format:           simplestreams.ProductFormat,
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
	cloud.Format = simplestreams.ProductFormat
	cloud.ContentId = ImageContentId
	cloud.Products = make(map[string]simplestreams.MetadataCatalog)
	itemsversion := updated.Format("20060102") // YYYYMMDD
	for _, t := range metadata {
		toWrite := *t
		// These fields are not required in the individual
		// record values - they are recorded at the top level.
		toWrite.RegionAlias = ""
		toWrite.Version = ""
		toWrite.Arch = ""
		if catalog, ok := cloud.Products[t.productId()]; ok {
			catalog.Items[itemsversion].Items[t.Id] = toWrite
		} else {
			catalog = simplestreams.MetadataCatalog{
				Arch:    t.Arch,
				Version: t.Version,
				Items: map[string]*simplestreams.ItemCollection{
					itemsversion: {
						Items: map[string]interface{}{t.Id: toWrite},
					},
				},
			}
			cloud.Products[t.productId()] = catalog
		}
	}
	return json.MarshalIndent(&cloud, "", "    ")
}
