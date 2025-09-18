// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
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
	}

	// Act
	obtained := transformOfferURLs(in)

	// Assert
	c.Assert(obtained, tc.DeepEquals, []string{
		"/qualifier/model",
		adminOfferURL,
		"/modelname.offername",
		crossmodel.MakeURL("fred-smith-canonical-com", "modelname", "offername", ""),
	})
}
