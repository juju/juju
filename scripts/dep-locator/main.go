package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juju/collections/set"
)

func main() {
	jujuDir := os.Args[1]
	if _, err := os.Stat(jujuDir); os.IsNotExist(err) {
		log.Fatalln(err)
	}
	packageDir := os.Args[2]
	dir := filepath.Join(jujuDir, packageDir)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		log.Fatalln(err)
	}

	cmd := exec.Command("go", "list", "-json")
	cmd.Dir = dir
	stdout, err := cmd.Output()
	if err != nil {
		log.Fatalln(err)
	}

	var list GoList
	if err := json.Unmarshal(stdout, &list); err != nil {
		log.Fatalln(err)
	}

	var packages []string
	for _, d := range list.Deps {
		if !strings.HasPrefix(d, "github.com/juju/juju") {
			continue
		}
		if strings.HasPrefix(d, "github.com/juju/juju/database") {
			fmt.Println("Found database package. Skipping...")
			continue
		}
		packages = append(packages, d)
	}

	checkPackages(jujuDir, packages, set.NewStrings(), "")
}

func checkPackages(jujuDir string, packages []string, ignore set.Strings, indent string) {
	s := set.NewStrings()
	for _, p := range packages {
		if ignore.Contains(p) {
			continue
		}
		path := strings.TrimPrefix(p, "github.com/juju/juju/")
		if path == "" {
			continue
		}

		dir := filepath.Join(jujuDir, path)

		fmt.Println(indent+"Checking", dir)

		cmd := exec.Command("go", "list", "-json")
		cmd.Dir = dir
		stdout, err := cmd.Output()
		if err != nil {
			log.Fatalln(err)
		}

		var list GoList
		if err := json.Unmarshal(stdout, &list); err != nil {
			log.Fatalln(err)
		}

		for _, d := range list.Deps {
			if strings.HasPrefix(d, "github.com/juju/juju/database") {
				fmt.Printf(indent+"ARGH!!! FOUND DATABASE PACKAGE IN %s\n", path)
				s.Add(p)
				break
			}
		}
	}

	if s.Size() == 0 {
		return
	}

	fmt.Println("Found packages with database import:", s.SortedValues())

	checkPackages(jujuDir, s.SortedValues(), set.NewStrings(packages...), indent+"    ")
}

type GoList struct {
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
