// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth_test

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/microsoft/kiota-abstractions-go/store"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/azure/internal/azureauth"
)

type ErrorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ErrorSuite{})

func (ErrorSuite) TestAsDataError(c *gc.C) {
	dataErr := odataerrors.NewODataError()
	dataErr.SetBackingStore(store.NewInMemoryBackingStore())
	me := odataerrors.NewMainError()
	me.SetCode(to.Ptr("code"))
	me.SetMessage(to.Ptr("message"))
	dataErr.SetErrorEscaped(me)

	de, ok := azureauth.AsDataError(dataErr)
	c.Assert(ok, jc.IsTrue)
	c.Assert(de.Error(), gc.Equals, "code: message")

	_, ok = azureauth.AsDataError(nil)
	c.Assert(ok, jc.IsFalse)

	azDataErr := &azureauth.DataError{}
	de, ok = azureauth.AsDataError(azDataErr)
	c.Assert(ok, jc.IsTrue)
	c.Assert(de, jc.DeepEquals, azDataErr)
}
