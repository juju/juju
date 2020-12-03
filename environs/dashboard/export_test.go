// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dashboard

import (
	"github.com/juju/juju/environs/simplestreams"
)

var (
	StreamsVersion = streamsVersion
	DownloadType   = downloadType
)

func NewConstraint(stream string, majorVersion int) *constraint {
	return &constraint{
		LookupParams: simplestreams.LookupParams{Stream: stream},
		majorVersion: majorVersion,
	}
}
