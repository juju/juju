// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import "github.com/juju/juju/worker/jwtparser"

func NewJWTParserGetterWrapper(getter jwtparser.Getter) jwtParserGetterWrapper {
	return jwtParserGetterWrapper{
		getter: getter,
	}
}
