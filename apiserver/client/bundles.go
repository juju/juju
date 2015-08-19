// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"strings"

	"github.com/juju/bundlechanges"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/params"
)

// GetBundleChanges returns the list of changes required to deploy the given
// bundle data. The changes are sorted by requirements, so that they can be
// applied in order.
func (c *Client) GetBundleChanges(args params.GetBundleChanges) (params.GetBundleChangesResults, error) {
	var results params.GetBundleChangesResults
	data, err := charm.ReadBundleData(strings.NewReader(args.YAML))
	if err != nil {
		return results, errors.Annotate(err, "cannot read bundle YAML")
	}
	// TODO frankban: provide a verifyConstraints function.
	if err := data.Verify(nil); err != nil {
		if err, ok := err.(*charm.VerificationError); ok {
			results.Errors = make([]string, len(err.Errors))
			for i, e := range err.Errors {
				results.Errors[i] = e.Error()
			}
			return results, nil
		}
		// This should never happen as Verify only returns verification errors.
		return results, errors.Annotate(err, "cannot verify bundle")
	}
	results.Changes = bundlechanges.FromData(data)
	return results, nil
}
