// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

func TestLegacySuite(t *testing.T) {
	tc.Run(t, &legacySuite{})
}

type legacySuite struct {
}

func (*legacySuite) TestTransformOfferURLs(c *tc.C) {
	// Arrange
	// Create a user tag with a domain
	userTag := names.NewUserTag("Fred.Smith@canonical.com")
	domainUserOfferURL := crossmodel.MakeURL(userTag.Id(), "modelname", "offername", "")
	domainUserOfferURLWithTag := crossmodel.MakeURL(userTag.String(), "modelname", "offername", "")

	// Create an Offer URL which will fail to parse.
	failToParseOfferURLStr := "/qualifier/model"
	_, err := crossmodel.ParseOfferURL(failToParseOfferURLStr)
	c.Check(err, tc.ErrorMatches, "offer URL is missing the name")

	// Create an Offer URL using a typical juju username.
	modelQualifier := model.QualifierFromUserTag(names.NewUserTag("admin"))
	adminOfferURL := crossmodel.MakeURL(modelQualifier.String(), "modelname", "offername", "")

	in := []string{
		// a bad offer URL, fails to parse, no transformation.
		failToParseOfferURLStr,
		// offer URL with model qualifier
		adminOfferURL,
		// offer URL with no model qualifier, no transformation.
		"/modelname.offername",
		// offer URL with external user
		domainUserOfferURL,
		// offer URL with owner tag qualifier
		domainUserOfferURLWithTag,
	}

	// Act
	obtained := transformOfferURLs(in)

	// Assert
	c.Assert(obtained, tc.DeepEquals, []string{
		"/qualifier/model",
		adminOfferURL,
		"/modelname.offername",
		crossmodel.MakeURL("fred-smith-canonical-com", "modelname", "offername", ""),
		crossmodel.MakeURL("fred-smith-canonical-com", "modelname", "offername", ""),
	})
}

func (*legacySuite) TestLegacyFiltersToFilters(c *tc.C) {
	in := params.OfferFiltersLegacy{
		Filters: []params.OfferFilterLegacy{
			{
				OwnerName: "fred.smith@canonical.com",
				ModelName: "model-a",
				OfferName: "offer-a",
			},
			{
				OwnerName: "fred-smith-canonical-com",
				ModelName: "model-b",
				OfferName: "offer-b",
			},
			{
				OwnerName: "user-fred.smith@canonical.com",
				ModelName: "model-c",
				OfferName: "offer-c",
			},
			{
				OwnerName: "bad/name",
				ModelName: "model-d",
				OfferName: "offer-d",
			},
		},
	}

	out := legacyFiltersToFilters(in)

	c.Assert(out, tc.DeepEquals, params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				ModelQualifier: "fred-smith-canonical-com",
				ModelName:      "model-a",
				OfferName:      "offer-a",
			},
			{
				ModelQualifier: "fred-smith-canonical-com",
				ModelName:      "model-b",
				OfferName:      "offer-b",
			},
			{
				ModelQualifier: "fred-smith-canonical-com",
				ModelName:      "model-c",
				OfferName:      "offer-c",
			},
			{
				ModelQualifier: "bad/name",
				ModelName:      "model-d",
				OfferName:      "offer-d",
			},
		},
	})
}
