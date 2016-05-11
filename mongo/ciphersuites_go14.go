// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
//
// +build !go1.5

package mongo

import (
	"crypto/tls"
)

var cipherSuites = []uint16{
	tls.TLS_RSA_WITH_AES_256_CBC_SHA,
}
