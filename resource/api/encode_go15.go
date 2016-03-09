// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//+build !go1.6

package api

func getEncoder() encoder {
	return bEncoding
}

func getDecoder() decoder {
	return &wordDecoder{}
}
