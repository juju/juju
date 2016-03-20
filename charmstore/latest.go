// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
)

// LatestCharmInfo returns the most up-to-date information about each
// of the identified charms at their latest revision. The revisions in
// the provided URLs are ignored.
func LatestCharmInfo(client BaseClient, cURLs []*charm.URL) ([]CharmInfoResult, error) {
	now := time.Now().UTC()

	// Do a bulk call to get the revision info for all charms.
	logger.Infof("retrieving revision information for %d charms", len(cURLs))
	revResults, err := client.LatestRevisions(cURLs)
	if err != nil {
		err = errors.Annotate(err, "while getting latest charm revision info")
		logger.Infof(err.Error())
		return nil, err
	}

	// Do a bulk call to get the latest info for each charm's resources.
	// TODO(ericsnow) Only do this for charms that *have* resources.
	chResources, err := client.ListResources(cURLs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Extract the results.
	var results []CharmInfoResult
	for i, cURL := range cURLs {
		revResult := revResults[i]
		resources := chResources[i]

		var result CharmInfoResult
		result.OriginalURL = cURL
		result.Timestamp = now
		if revResult.Err != nil {
			result.Error = errors.Trace(revResult.Err)
		} else {
			result.LatestRevision = revResult.Revision
			result.LatestResources = resources
		}
		results = append(results, result)
	}
	return results, nil
}
