// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build cover

package coveruploader

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime/coverage"
	"time"
	_ "unsafe"
)

// putURL is injected by build scripts as the target for the
// uploaded coverage profile.
var putURL string

func Enable() {
	if putURL == "" {
		return
	}
	ticker := time.NewTicker(10 * time.Second)
	f := func() {
		ticker.Stop()
		upload()
	}
	go func() {
		for {
			_, ok := <-ticker.C
			if !ok {
				return
			}
			upload()
		}
	}()
	addExitHook(hook{
		F:            f,
		RunOnFailure: true,
	})
}

func upload() {
	cm := &bytes.Buffer{}
	err := coverage.WriteMeta(cm)
	if err != nil {
		log.Printf("cover: cannot write coverage meta: %v\n", err)
		return
	}
	cc := &bytes.Buffer{}
	err = coverage.WriteCounters(cc)
	if err != nil {
		log.Printf("cover: cannot write coverage counters: %v\n", err)
		return
	}

	var randName [16]byte
	_, _ = rand.Read(randName[:])
	name := hex.EncodeToString(randName[:])

	tb := &bytes.Buffer{}
	gw := gzip.NewWriter(tb)
	tw := tar.NewWriter(gw)
	err = tw.WriteHeader(&tar.Header{
		Name:     fmt.Sprintf("covmeta.%s", name),
		Size:     int64(cm.Len()),
		Mode:     0644,
		Typeflag: tar.TypeReg,
	})
	if err != nil {
		log.Printf("cover: cannot write coverage meta tar header: %v\n", err)
		return
	}
	_, err = io.Copy(tw, cm)
	if err != nil {
		log.Printf("cover: cannot write coverage meta body: %v\n", err)
		return
	}
	err = tw.WriteHeader(&tar.Header{
		Name:     fmt.Sprintf("covcounters.%s.0.%d", name, time.Now().UnixNano()),
		Size:     int64(cc.Len()),
		Mode:     0644,
		Typeflag: tar.TypeReg,
	})
	if err != nil {
		log.Printf("cover: cannot write coverage counters tar header: %v\n", err)
		return
	}
	_, err = io.Copy(tw, cc)
	if err != nil {
		log.Printf("cover: cannot write coverage counters body: %v\n", err)
		return
	}
	_ = tw.Close()
	_ = gw.Close()

	req, err := http.NewRequest("PUT", putURL, tb)
	if err != nil {
		log.Printf("cover: cannot upload cover profile to %q: %v\n", putURL, err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("cover: cannot upload cover profile to %q: %v\n", putURL, err)
		return
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("cover: cannot upload cover profile to %q: %v\n", putURL, err)
		return
	}
	if resp.StatusCode >= 400 {
		log.Printf("cover: failed to upload cover profile to %q with %d:\n%s",
			putURL, resp.StatusCode, respBody)
		return
	}
}

// Exit hooks from go src/internal/runtime/exithook/hooks.go

//go:linkname addExitHook internal/runtime/exithook.Add
func addExitHook(h hook)

// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
type hook struct {
	F            func() // func to run
	RunOnFailure bool   // whether to run on non-zero exit code
}
