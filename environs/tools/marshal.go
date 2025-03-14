// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/environs/simplestreams"
)

// ToolsContentId returns the tools content id for the given stream.
func ToolsContentId(stream string) string {
	return fmt.Sprintf("com.ubuntu.juju:%s:agents", stream)
}

// ProductMetadataPath returns the tools product metadata path for the given stream.
func ProductMetadataPath(stream string) string {
	return fmt.Sprintf("streams/v1/com.ubuntu.juju-%s-agents.json", stream)
}

// MarshalToolsMetadataJSON marshals tools metadata to index and products JSON.
// updated is the time at which the JSON file was updated.
func MarshalToolsMetadataJSON(metadata map[string][]*ToolsMetadata, updated time.Time) (index, legacyIndex []byte, products map[string][]byte, err error) {
	if index, legacyIndex, err = marshalToolsMetadataIndexJSON(context.TODO(), metadata, updated); err != nil {
		return nil, nil, nil, err
	}
	if products, err = MarshalToolsMetadataProductsJSON(metadata, updated); err != nil {
		return nil, nil, nil, err
	}
	return index, legacyIndex, products, err
}

// marshalToolsMetadataIndexJSON marshals tools metadata to index JSON.
// updated is the time at which the JSON file was updated.
func marshalToolsMetadataIndexJSON(ctx context.Context, streamMetadata map[string][]*ToolsMetadata, updated time.Time) (out, outlegacy []byte, err error) {
	var indices simplestreams.Indices
	indices.Updated = updated.Format(time.RFC1123Z)
	indices.Format = simplestreams.IndexFormat
	indices.Indexes = make(map[string]*simplestreams.IndexMetadata, len(streamMetadata))

	var indicesLegacy simplestreams.Indices
	indicesLegacy.Updated = updated.Format(time.RFC1123Z)
	indicesLegacy.Format = simplestreams.IndexFormat

	for stream, metadata := range streamMetadata {
		var productIds []string
		for _, t := range metadata {
			id, err := t.productId()
			if err != nil {
				if errors.Is(err, errors.NotValid) {
					logger.Infof(ctx, "ignoring tools metadata with unknown os type %q", t.Release)
					continue
				}
				return nil, nil, err
			}
			productIds = append(productIds, id)
		}
		indexMetadata := &simplestreams.IndexMetadata{
			Updated:          indices.Updated,
			Format:           simplestreams.ProductFormat,
			DataType:         ContentDownload,
			ProductsFilePath: ProductMetadataPath(stream),
			ProductIds:       set.NewStrings(productIds...).SortedValues(),
		}
		indices.Indexes[ToolsContentId(stream)] = indexMetadata
		if stream == ReleasedStream {
			indicesLegacy.Indexes = make(map[string]*simplestreams.IndexMetadata, 1)
			indicesLegacy.Indexes[ToolsContentId(stream)] = indexMetadata
		}
	}
	out, err = json.MarshalIndent(&indices, "", "    ")
	if len(indicesLegacy.Indexes) == 0 {
		return out, nil, err
	}
	outlegacy, err = json.MarshalIndent(&indicesLegacy, "", "    ")
	if err != nil {
		return nil, nil, err
	}
	return out, outlegacy, nil
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
				if errors.Is(err, errors.NotValid) {
					continue
				}
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
						itemsversion: {
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
