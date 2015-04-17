// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package upgrades_test

var customImageMetadata = map[string][]byte{
	"images/abc":     []byte("abc"),
	"images/def/ghi": []byte("xyz"),
}
