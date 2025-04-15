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
	"os"
	"runtime/coverage"
	"time"
	_ "unsafe"
)

// init silences the GOCOVERDIR warning by setting the GOCOVERDIR env var
// to a temporary directory.
func init() {
	if putURL == "" {
		return
	}
	if os.Getenv("GOCOVERDIR") != "" {
		return
	}
	tempDir, err := os.MkdirTemp("", "gocover")
	if err != nil {
		return
	}
	os.Setenv("GOCOVERDIR", tempDir)
}

// putURL is injected by build scripts as the target for the
// uploaded coverage profile.
var putURL string

// Enable the coveruploader for this process. Periodically and on process
// exit, cover profiles are uploaded via a HTTP PUT to the URL set on putURL.
// The putURL could be a pre-signed URL to an S3 bucket with versioning
// enabled.
func Enable() {
	if putURL == "" {
		log.Printf("cover: cover profiles are enabled but no putURL supplied at build")
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
