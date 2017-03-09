// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//+build go1.6

package api

import "mime"

// XXX
// TODO(natefinch) remove this once we support building on go 1.6 for all platforms.

// formatMediaType serializes mediatype t and the parameters
// param as a media type conforming to RFC 2045 and RFC 2616.
// The type and parameter names are written in lower-case.
// When any of the arguments result in a standard violation then
// formatMediaType returns the empty string.
func formatMediaType(t string, param map[string]string) string {
	return mime.FormatMediaType(t, param)
}

// ParseMediaType parses a media type value and any optional
// parameters, per RFC 1521.  Media types are the values in
// Content-Type and Content-Disposition headers (RFC 2183).
// On success, ParseMediaType returns the media type converted
// to lowercase and trimmed of white space and a non-nil map.
// The returned map, params, maps from the lowercase
// attribute to the attribute value with its case preserved.
func ParseMediaType(v string) (mediatype string, params map[string]string, err error) {
	return mime.ParseMediaType(v)
}

func getEncoder() encoder {
	return mime.BEncoding
}

func getDecoder() decoder {
	return &mime.WordDecoder{}
}
