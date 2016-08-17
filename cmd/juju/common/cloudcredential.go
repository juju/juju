// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"strings"

	"github.com/juju/errors"

	"gopkg.in/juju/names.v2"
)

// ResolveCloudCredentialTag takes a string which is of either the format
// "<credential>" or "<user>/<credential>". If the string does not include
// a user, then the supplied user tag is implied.
func ResolveCloudCredentialTag(user names.UserTag, cloud names.CloudTag, credentialName string) (names.CloudCredentialTag, error) {
	if i := strings.IndexRune(credentialName, '/'); i == -1 {
		credentialName = fmt.Sprintf("%s/%s", user.Id(), credentialName)
	}
	s := fmt.Sprintf("%s/%s", cloud.Id(), credentialName)
	if !names.IsValidCloudCredential(s) {
		return names.CloudCredentialTag{}, errors.NotValidf("cloud credential name %q", s)
	}
	return names.NewCloudCredentialTag(s), nil
}
