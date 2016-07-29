/*
Copyright (c) 2014 VMware, Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package disk

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	B   = 1
	KiB = 1024 * B
	MiB = 1024 * KiB
	GiB = 1024 * MiB
	TiB = 1024 * GiB
	PiB = 1024 * TiB
)

type ByteValue struct {
	Bytes int64
}

func (b *ByteValue) String() string {
	v := b.Bytes
	suffix := "B"

	for _, s := range []string{"K", "M", "G", "T", "P"} {
		if v < 1024 {
			break
		}

		suffix = fmt.Sprintf("%siB", s)
		v /= 1024
	}

	return fmt.Sprintf("%d%s", v, suffix)
}

var bytesRegexp = regexp.MustCompile(`^(?i)(\d+)([KMGTP]?)(ib|b)?$`)

func (b *ByteValue) Set(s string) error {
	m := bytesRegexp.FindStringSubmatch(s)
	if len(m) == 0 {
		return errors.New("invalid byte value")
	}

	v32, _ := strconv.Atoi(m[1])
	v := int64(v32)
	switch strings.ToUpper(m[2]) {
	case "K":
		v *= 1024
	case "M":
		v *= 1024 * 1024
	case "G":
		v *= 1024 * 1024 * 1024
	case "T":
		v *= 1024 * 1024 * 1024 * 1024
	case "P":
		v *= 1024 * 1024 * 1024 * 1024 * 1024
	case "E":
		v *= 1024 * 1024 * 1024 * 1024 * 1024 * 1024
	}

	b.Bytes = v
	return nil
}
