// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

func toValue[T any](v *T) T {
	if v == nil {
		return *new(T)
	}
	return *v
}

func toMapPtr(in map[string]string) map[string]*string {
	result := make(map[string]*string)
	for k, v := range in {
		result[k] = to.Ptr(v)
	}
	return result
}

type keyBundle struct {
	Key *jsonWebKey `json:"key"`
}

type jsonWebKey struct {
	Kid *string `json:"kid"`
	Kty string  `json:"kty"`
}
