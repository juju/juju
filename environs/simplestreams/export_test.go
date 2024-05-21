// Copyright 2013,2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplestreams

import jujuhttp "github.com/juju/juju/internal/http"

func ExtractCatalogsForProducts(metadata CloudMetadata, productIds []string) []MetadataCatalog {
	return metadata.extractCatalogsForProducts(productIds)
}

func ExtractIndexes(ind Indices, ids []string) IndexMetadataSlice {
	return ind.extractIndexes(ids)
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

var FetchData = fetchData

func HttpClient(ds DataSource) *jujuhttp.Client {
	urlds, ok := ds.(*urlDataSource)
	if ok {
		return urlds.httpClient
	}
	return nil
}
