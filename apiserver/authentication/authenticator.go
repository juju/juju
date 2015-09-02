// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
)

// AuthenticatorForTag looks up the authenticator for the given tag.
func AuthenticatorForTag(s string, authenticators map[string]EntityAuthenticator) (EntityAuthenticator, names.Tag, error) {
	key := ""
	tag := names.Tag(nil)
	if s != "" {
		var err error
		tag, err = names.ParseTag(s)
		if err != nil {
			return nil, nil, errors.Annotate(err, "failed to determine the tag kind")
		}
		key = tag.Kind()
	}
	authenticator, ok := authenticators[key]
	if !ok {
		return nil, nil, common.ErrBadRequest
	}
	return authenticator, tag, nil
}
