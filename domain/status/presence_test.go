// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/status"
)

type presenceTokenSuite struct{}

func TestPresenceTokenSuite(t *testing.T) {
	tc.Run(t, presenceTokenSuite{})
}

// TestNewPresenceToken verifies that a newly generated presence token is
// non-empty and valid.
func (presenceTokenSuite) TestNewPresenceToken(c *tc.C) {
	token, err := status.NewPresenceToken()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(token.IsEmpty(), tc.IsFalse)
	c.Assert(token.Validate(), tc.ErrorIsNil)
}

// TestPresenceTokenValidate checks valid, empty, and malformed presence tokens.
func (presenceTokenSuite) TestPresenceTokenValidate(c *tc.C) {
	const validUUID = "0de7ed80-bfcf-49b1-876a-31462e940ca1"

	tests := []struct {
		name      string
		token     status.PresenceToken
		expectErr error
	}{
		{
			name:  "valid uuid",
			token: status.PresenceToken(validUUID),
		},
		{
			name:      "empty token",
			token:     "",
			expectErr: coreerrors.NotValid,
		},
		{
			name:      "invalid uuid",
			token:     "not-a-uuid",
			expectErr: coreerrors.NotValid,
		},
	}

	for _, test := range tests {
		c.Logf("case: %s", test.name)
		err := test.token.Validate()
		if test.expectErr == nil {
			c.Assert(err, tc.ErrorIsNil)
			continue
		}
		c.Assert(err, tc.ErrorIs, test.expectErr)
	}
}

// TestPresenceTokenString verifies that String returns the raw token string.
func (presenceTokenSuite) TestPresenceTokenString(c *tc.C) {
	const validUUID = "0de7ed80-bfcf-49b1-876a-31462e940ca1"
	c.Check(status.PresenceToken(validUUID).String(), tc.Equals, validUUID)
}

// TestPresenceTokenIsEmpty verifies empty and non-empty presence tokens.
func (presenceTokenSuite) TestPresenceTokenIsEmpty(c *tc.C) {
	c.Check(status.PresenceToken("").IsEmpty(), tc.IsTrue)
	c.Check(status.PresenceToken("0de7ed80-bfcf-49b1-876a-31462e940ca1").IsEmpty(), tc.IsFalse)
}
