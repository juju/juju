// Copyright 2015 CoreOS, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package systemd

import (
	"bytes"
	"io"

	"github.com/coreos/go-systemd/unit"
)

// UnitSerialize encodes all of the given UnitOption objects into a unit file.
// Renamed from Serialize from github.com/coreos/go-systemd/unit so as to not
// confict with the exported internal function in export_test.go.
func UnitSerialize(opts []*unit.UnitOption) io.Reader {
	var buf bytes.Buffer

	if len(opts) == 0 {
		return &buf
	}

	idx := map[string][]*unit.UnitOption{}
	for _, opt := range opts {
		idx[opt.Section] = append(idx[opt.Section], opt)
	}

	// CHANGED HERE: Output in the following order:
	// - Unit
	// - Service
	// - Install
	// rather than just iterating over the map in random order.
	for _, curSection := range []string{"Unit", "Service", "Install"} {
		curOpts, found := idx[curSection]
		if !found {
			continue
		}
		writeSectionHeader(&buf, curSection)
		writeNewline(&buf)

		for _, opt := range curOpts {
			writeOption(&buf, opt)
			writeNewline(&buf)
		}
		writeNewline(&buf)
	}

	return &buf
}

func writeNewline(buf *bytes.Buffer) {
	buf.WriteRune('\n')
}

func writeSectionHeader(buf *bytes.Buffer, section string) {
	buf.WriteRune('[')
	buf.WriteString(section)
	buf.WriteRune(']')
}

func writeOption(buf *bytes.Buffer, opt *unit.UnitOption) {
	buf.WriteString(opt.Name)
	buf.WriteRune('=')
	buf.WriteString(opt.Value)
}
