// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

// AssertProviderAuthTypes asserts that the given provider has credential
// schemas for exactly the specified set of authentication types.
func AssertProviderAuthTypes(c *gc.C, p environs.EnvironProvider, expectedAuthTypes ...cloud.AuthType) {
	var authTypes []cloud.AuthType
	for authType := range p.CredentialSchemas() {
		authTypes = append(authTypes, authType)
	}
	c.Assert(authTypes, jc.SameContents, expectedAuthTypes)
}

// AssertProviderCredentialsValid asserts that the given provider is
// able to validate the given authentication type and credential
// attributes; and that removing any one of the attributes will cause
// the validation to fail.
func AssertProviderCredentialsValid(c *gc.C, p environs.EnvironProvider, authType cloud.AuthType, attrs map[string]string) {
	schema, ok := p.CredentialSchemas()[authType]
	c.Assert(ok, jc.IsTrue, gc.Commentf("missing schema for %q auth-type", authType))
	validate := func(attrs map[string]string) error {
		_, err := schema.Finalize(attrs, func(string) ([]byte, error) {
			return nil, errors.NotSupportedf("reading files")
		})
		return err
	}

	err := validate(attrs)
	c.Assert(err, jc.ErrorIsNil)

	for excludedKey := range attrs {
		reducedAttrs := make(map[string]string)
		for key, value := range attrs {
			if key != excludedKey {
				reducedAttrs[key] = value
			}
		}
		err := validate(reducedAttrs)
		field := schema[excludedKey]
		if field.FileAttr != "" {
			c.Assert(err, gc.ErrorMatches, fmt.Sprintf(
				`either %q or %q must be specified`, excludedKey, field.FileAttr),
			)
		} else {
			c.Assert(err, gc.ErrorMatches, excludedKey+": expected string, got nothing")
		}
	}
}

// AssertProviderCredentialsAttributesHidden asserts that the provider
// credentials schema for the given provider and authentication type
// marks the specified attributes (and only those attributes) as being
// hidden.
func AssertProviderCredentialsAttributesHidden(c *gc.C, p environs.EnvironProvider, authType cloud.AuthType, expectedHidden ...string) {
	var hidden []string
	schema, ok := p.CredentialSchemas()[authType]
	c.Assert(ok, jc.IsTrue, gc.Commentf("missing schema for %q auth-type", authType))
	for key, field := range schema {
		if field.Hidden {
			hidden = append(hidden, key)
		}
	}
	c.Assert(hidden, jc.SameContents, expectedHidden)
}
