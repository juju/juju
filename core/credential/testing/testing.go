// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corecredential "github.com/juju/juju/core/credential"
)

// GenCredentialUUID can be used in testing for generating a credential uuid
// that is checked for subsequent errors using the test suits go check instance.
func GenCredentialUUID(c *gc.C) corecredential.UUID {
	uuid, err := corecredential.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return uuid
}
