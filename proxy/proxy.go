// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxy

type Proxier interface {
	Port() string
	Start() error
	Type() string
}
