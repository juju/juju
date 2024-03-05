// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"encoding/base64"
	"fmt"
	"sync/atomic"

	"github.com/aws/smithy-go"
)

var b64 = base64.StdEncoding

type counter struct {
	value int32
}

func (c *counter) next() int {
	i := atomic.AddInt32(&c.value, 1)
	return int(i - 1)
}

func (c *counter) get() (i int) {
	return int(atomic.LoadInt32(&c.value))
}

func (c *counter) reset() {
	atomic.StoreInt32(&c.value, 0)
}

func apiError(code string, f string, a ...interface{}) smithy.APIError {
	return &smithy.GenericAPIError{
		Code:    code,
		Message: fmt.Sprintf(f, a...),
	}
}
