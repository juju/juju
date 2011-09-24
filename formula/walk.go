// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package formula

import (
	"os"
	"path/filepath"
	"sort"
)

// Drop in replacement for filepath.Walk while issue 2237 is not solved:
//
//    http://code.google.com/p/go/issues/detail?id=2237

type walkErrorHandler interface {
	Error(path string, err os.Error)
}

// walk walks the file tree rooted at root, calling v.VisitDir or
// v.VisitFile for each directory or file in the tree, including root.
// If v.VisitDir returns false, Walk skips the directory's entries;
// otherwise it invokes itself for each directory entry in sorted order.
// If the visitor implements the walkErrorHandler interface, any errors
// found will be dispatched to it.
func walk(root string, v filepath.Visitor) {
	f, err := os.Lstat(root)
	if err != nil {
		if eh, ok := v.(walkErrorHandler); ok {
			eh.Error(root, err)
		}
		return // can't progress
	}
	walk_(root, f, v)
}

func walk_(path string, f *os.FileInfo, v filepath.Visitor) {
	if !f.IsDirectory() {
		v.VisitFile(path, f)
		return
	}
	if !v.VisitDir(path, f) {
		return // skip directory entries
	}
	list, err := readDir(path)
	if err != nil {
		if eh, ok := v.(walkErrorHandler); ok {
			eh.Error(path, err)
		}
	}
	for _, e := range list {
		walk_(filepath.Join(path, e.Name), e, v)
	}
}

func readDir(dirname string) ([]*os.FileInfo, os.Error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	list, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		return nil, err
	}
	fi := make(fileInfoList, len(list))
	for i := range list {
		fi[i] = &list[i]
	}
	sort.Sort(fi)
	return fi, nil
}

type fileInfoList []*os.FileInfo

func (f fileInfoList) Len() int           { return len(f) }
func (f fileInfoList) Less(i, j int) bool { return f[i].Name < f[j].Name }
func (f fileInfoList) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }
