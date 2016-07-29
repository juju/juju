// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package usso

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Remove the standard ports from the URL.
func normalizeHost(scheme, hostSpec string) string {
	standardPorts := map[string]string{
		"http":  "80",
		"https": "443",
	}
	hostParts := strings.Split(hostSpec, ":")
	if len(hostParts) == 2 && hostParts[1] == standardPorts[scheme] {
		// There's a port, but it's the default one.  Leave it out.
		return hostParts[0]
	}
	return hostSpec
}

// Normalize the URL according to OAuth specs.
func NormalizeURL(inputUrl string) (string, error) {
	parsedUrl, err := url.Parse(inputUrl)
	if err != nil {
		return "", err
	}

	host := normalizeHost(parsedUrl.Scheme, parsedUrl.Host)
	normalizedUrl := fmt.Sprintf(
		"%v://%v%v", parsedUrl.Scheme, host, parsedUrl.Path)
	return normalizedUrl, nil
}

type parameterSlice []parameter

func (p parameterSlice) Len() int {
	return len(p)
}

func (p parameterSlice) Less(i, j int) bool {
	if p[i].key < p[j].key {
		return true
	}
	if p[i].key == p[j].key {
		return p[i].value < p[j].value
	}
	return false
}

func (p parameterSlice) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func (p parameterSlice) String() string {
	ss := make([]string, len(p))
	for i, param := range p {
		ss[i] = param.String()
	}
	return strings.Join(ss, "&")
}

type parameter struct {
	key, value string
}

func (p parameter) String() string {
	return fmt.Sprintf("%s=%s", p.key, p.value)
}

// Normalize the parameters in the query string according to
// http://tools.ietf.org/html/rfc5849#section-3.4.1.3.2.
// url.Values.Encode encoded the GET parameters in a consistent order we
// do the encoding ourselves.
func NormalizeParameters(parameters url.Values) (string, error) {
	var ps parameterSlice
	for k, vs := range parameters {
		if k == "oauth_signature" {
			continue
		}
		k = escape(k)
		for _, v := range vs {
			v = escape(v)
			ps = append(ps, parameter{k, v})
		}
	}
	sort.Sort(ps)
	return ps.String(), nil
}

var escaped = [4]uint64{
	0xFC009FFFFFFFFFFF,
	0xB800000178000001,
	0xFFFFFFFFFFFFFFFF,
	0xFFFFFFFFFFFFFFFF,
}

// escape percent encodes s as defined in
// http://tools.ietf.org/html/rfc5849#section-3.6.
//
// Note: this is slightly different from the output of url.QueryEscape.
func escape(s string) string {
	var count int
	for i := 0; i < len(s); i++ {
		if (escaped[s[i]>>6]>>(s[i]&0x3f))&1 == 1 {
			count++
		}
	}
	if count == 0 {
		return s
	}
	buf := make([]byte, len(s)+2*count)
	j := 0
	for i := 0; i < len(s); i++ {
		if (escaped[s[i]>>6]>>(s[i]&0x3f))&1 == 1 {
			buf[j] = '%'
			buf[j+1] = "0123456789ABCDEF"[s[i]>>4]
			buf[j+2] = "0123456789ABCDEF"[s[i]&0xf]
			j += 3
			continue
		}
		buf[j] = s[i]
		j++
	}
	return string(buf)
}
