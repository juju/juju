// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: import-inspector <package>")
	}
	cmd := exec.Command("go", "list", "-json", os.Args[1])
	stdout, err := cmd.Output()
	if err != nil {
		matches, err := filepath.Glob(filepath.Join(os.Args[1], "*.go"))
		if len(matches) == 0 && err == nil {
			fmt.Println(string("[]"))
			return
		}

		log.Fatalf("failed to run go list: %v", err.Error())
	}

	set := make(map[string]struct{})
	dec := json.NewDecoder(bytes.NewReader(stdout))
	for {
		var list List

		err := dec.Decode(&list)
		if err == io.EOF {
			// all done
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		for _, d := range list.Deps {
			set[d] = struct{}{}
		}
	}

	sorted := make([]string, 0, len(set))
	for p := range set {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)
	data, err := json.Marshal(sorted)
	if err != nil {
		log.Fatal(err.Error())
	}

	fmt.Fprintln(os.Stderr, "list of packages from ", os.Args[1])
	fmt.Println(string(data))
}

type List struct {
	Dir        string
	ImportPath string
	Name       string
	Target     string
	Stale      bool
	Root       string
	GoFiles    []string
	Imports    []string
	Deps       []string
}
