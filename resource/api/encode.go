// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//+build go1.6

package api

import "mime"

// TODO(natefinch) move this into a normal file once we support building on go 1.6 for all platforms.

func getEncoder() encoder {
	return mime.BEncoding
}

func getDecoder() decoder {
	return &mime.WordDecoder{}
}
