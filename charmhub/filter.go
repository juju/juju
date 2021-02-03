// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"fmt"
)

func appendFilterList(value string, filters []string) []string {
	retVals := make([]string, len(filters))
	for i, v := range filters {
		retVals[i] = fmt.Sprintf("%s.%s", value, v)
	}
	return retVals
}

var defaultChannelFilter = []string{
	"channel.name",
	"channel.platform.architecture",
	"channel.platform.os",
	"channel.platform.series",
	"channel.released-at",
	"channel.risk",
	"channel.track",
}

var defaultResultFilter = []string{
	"result.categories.featured",
	"result.categories.name",
	"result.contains-charms.name",
	"result.contains-charms.package-id",
	"result.contains-charms.store-url",
	"result.description",
	"result.license",
	"result.publisher.display-name",
	"result.store-url",
	"result.summary",
}

var defaultDownloadFilter = []string{
	"download.hash-sha-256",
	"download.size",
	"download.url",
}
