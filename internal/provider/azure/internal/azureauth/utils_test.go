// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth_test

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/juju/tc"
	"github.com/microsoft/kiota-abstractions-go/store"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"

	"github.com/juju/juju/internal/provider/azure/internal/azureauth"
	"github.com/juju/juju/internal/testhelpers"
)

type ErrorSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&ErrorSuite{})

func (ErrorSuite) TestAsDataError(c *tc.C) {
	dataErr := odataerrors.NewODataError()
	dataErr.SetBackingStore(store.NewInMemoryBackingStore())
	me := odataerrors.NewMainError()
	me.SetCode(to.Ptr("code"))
	me.SetMessage(to.Ptr("message"))
	dataErr.SetErrorEscaped(me)

	de, ok := azureauth.AsDataError(dataErr)
	c.Assert(ok, tc.IsTrue)
	c.Assert(de.Error(), tc.Equals, "code: message")

	_, ok = azureauth.AsDataError(nil)
	c.Assert(ok, tc.IsFalse)

	azDataErr := &azureauth.DataError{}
	de, ok = azureauth.AsDataError(azDataErr)
	c.Assert(ok, tc.IsTrue)
	c.Assert(de, tc.DeepEquals, azDataErr)
}
