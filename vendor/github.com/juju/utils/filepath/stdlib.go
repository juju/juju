// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.golang file.

package filepath

import (
	"strings"
)

// The following functions are adapted from the GO stdlib source.

// Base mimics path/filepath for the given path separator.
func Base(sep uint8, volumeName func(string) string, path string) string {
	if path == "" {
		return "."
	}
	// Strip trailing slashes.
	for len(path) > 0 && path[len(path)-1] == sep {
		path = path[0 : len(path)-1]
	}
	// Throw away volume name
	path = path[len(volumeName(path)):]
	// Find the last element
	i := len(path) - 1
	for i >= 0 && path[i] != sep {
		i--
	}
	if i >= 0 {
		path = path[i+1:]
	}
	// If empty now, it had only slashes.
	if path == "" {
		return string(sep)
	}
	return path
}

// A lazybuf is a lazily constructed path buffer.
// It supports append, reading previously appended bytes,
// and retrieving the final string. It does not allocate a buffer
// to hold the output until that output diverges from s.
type lazybuf struct {
	path       string
	buf        []byte
	w          int
	volAndPath string
	volLen     int
}

func (b *lazybuf) index(i int) byte {
	if b.buf != nil {
		return b.buf[i]
	}
	return b.path[i]
}

func (b *lazybuf) append(c byte) {
	if b.buf == nil {
		if b.w < len(b.path) && b.path[b.w] == c {
			b.w++
			return
		}
		b.buf = make([]byte, len(b.path))
		copy(b.buf, b.path[:b.w])
	}
	b.buf[b.w] = c
	b.w++
}

func (b *lazybuf) string() string {
	if b.buf == nil {
		return b.volAndPath[:b.volLen+b.w]
	}
	return b.volAndPath[:b.volLen] + string(b.buf[:b.w])
}

// Clean mimics path/filepath for the given path separator.
func Clean(sep uint8, volumeName func(string) string, path string) string {
	originalPath := path
	volLen := len(volumeName(path))
	path = path[volLen:]
	if path == "" {
		if volLen > 1 && originalPath[1] != ':' {
			// should be UNC
			return FromSlash(sep, originalPath)
		}
		return originalPath + "."
	}
	rooted := (path[0] == sep)

	// Invariants:
	//  reading from path; r is index of next byte to process.
	//  writing to buf; w is index of next byte to write.
	//  dotdot is index in buf where .. must stop, either because
	//      it is the leading slash or it is a leading ../../.. prefix.
	n := len(path)
	out := lazybuf{path: path, volAndPath: originalPath, volLen: volLen}
	r, dotdot := 0, 0
	if rooted {
		out.append(sep)
		r, dotdot = 1, 1
	}

	for r < n {
		switch {
		case path[r] == sep:
			// empty path element
			r++
		case path[r] == '.' && (r+1 == n || path[r+1] == sep):
			// . element
			r++
		case path[r] == '.' && path[r+1] == '.' && (r+2 == n || path[r+2] == sep):
			// .. element: remove to last separator
			r += 2
			switch {
			case out.w > dotdot:
				// can backtrack
				out.w--
				for out.w > dotdot && out.index(out.w) != sep {
					out.w--
				}
			case !rooted:
				// cannot backtrack, but not rooted, so append .. element.
				if out.w > 0 {
					out.append(sep)
				}
				out.append('.')
				out.append('.')
				dotdot = out.w
			}
		default:
			// real path element.
			// add slash if needed
			if rooted && out.w != 1 || !rooted && out.w != 0 {
				out.append(sep)
			}
			// copy element
			for ; r < n && path[r] != sep; r++ {
				out.append(path[r])
			}
		}
	}

	// Turn empty string into "."
	if out.w == 0 {
		out.append('.')
	}

	return FromSlash(sep, out.string())
}

// Dir mimics path/filepath for the given path separator.
func Dir(sep uint8, volumeName func(string) string, path string) string {
	vol := volumeName(path)
	i := len(path) - 1
	for i >= len(vol) && path[i] != sep {
		i--
	}
	dir := Clean(sep, volumeName, path[len(vol):i+1])
	return vol + dir
}

// Ext mimics path/filepath for the given path separator.
func Ext(sep uint8, path string) string {
	for i := len(path) - 1; i >= 0 && path[i] != sep; i-- {
		if path[i] == '.' {
			return path[i:]
		}
	}
	return ""
}

// FromSlash mimics path/filepath for the given path separator.
func FromSlash(sep uint8, path string) string {
	if sep == '/' {
		return path
	}
	return strings.Replace(path, "/", string(sep), -1)
}

// Join mimics path/filepath for the given path separator.
func Join(sep uint8, volumeName func(string) string, elem ...string) string {
	for i, e := range elem {
		if e != "" {
			return Clean(sep, volumeName, strings.Join(elem[i:], string(sep)))
		}
	}
	return ""
}

// Split mimics path/filepath for the given path separator.
func Split(sep uint8, volumeName func(string) string, path string) (dir, file string) {
	vol := volumeName(path)
	i := len(path) - 1
	for i >= len(vol) && path[i] != sep {
		i--
	}
	return path[:i+1], path[i+1:]
}

// ToSlash mimics path/filepath for the given path separator.
func ToSlash(sep uint8, path string) string {
	if sep == '/' {
		return path
	}
	return strings.Replace(path, string(sep), "/", -1)
}
