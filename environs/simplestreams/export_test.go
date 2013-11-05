// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

func ExtractCatalogsForProducts(metadata CloudMetadata, productIds []string) []MetadataCatalog {
	return metadata.extractCatalogsForProducts(productIds)
}

func ExtractIndexes(ind Indices) IndexMetadataSlice {
	return ind.extractIndexes()
}

func HasCloud(metadata IndexMetadata, cloud CloudSpec) bool {
	return metadata.hasCloud(cloud)
}

func HasProduct(metadata IndexMetadata, prodIds []string) bool {
	return metadata.hasProduct(prodIds)
}

func Filter(entries IndexMetadataSlice, match func(*IndexMetadata) bool) IndexMetadataSlice {
	return entries.filter(match)
}

func SetSeriesVersions(value map[string]string) func() {
	origVersions := seriesVersions
	origUpdated := updatedseriesVersions
	seriesVersions = value
	updatedseriesVersions = false
	return func() {
		seriesVersions = origVersions
		updatedseriesVersions = origUpdated
	}
}
