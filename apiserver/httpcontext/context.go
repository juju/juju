// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpcontext

import "context"

// EntityForContext is responsible for taking a regular context and determining
// the entity that is associated with the context. Entity is purposely left
// vague in this context as it could resemble multiple things. It is not
// expected that the caller will try and interpret the string returned. The
// value should only be used for information display context.
//
// If no entity can be established for the current context then "unknown" is
// returned. It is expected the context used here comes from a http request.
func EntityForContext(ctx context.Context) string {
	authInfo, exists := RequestAuthInfo(ctx)
	if !exists {
		return "unknown"
	}

	return authInfo.Tag.Id()
}
