package main

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

func main() {
	path := os.Args[1]
	fullVersion := os.Args[2]
	parts := strings.Split(fullVersion, "-")
	if len(parts) < 3 {
		log.Fatalf("expected number of parts to be greater than 3")
	}

	arch := parts[len(parts)-1]
	series := parts[len(parts)-2]
	version := strings.Join(parts[:len(parts)-2], "-")

	var files []string
	if err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(info.Name()) == ".tmpl" {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	buf, err := ioutil.ReadFile(filepath.Join(path, "tools", "agent", fullVersion+".tar.gz"))
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		tmpl := template.Must(template.ParseFiles(file))

		f, err := os.Create(file[:len(file)-5])
		if err != nil {
			log.Fatal(err)
		}

		err = tmpl.Execute(f, []struct {
			Version string
			Arch    string
			Series  string
			Size    int
			Path    string
			SHA256  string
			Date    string
		}{{
			Version: version,
			Arch:    arch,
			Series:  series,
			Size:    len(buf),
			Path:    filepath.Join("agent", fullVersion+".tar.gz"),
			SHA256:  fmt.Sprintf("%x", sha256.Sum256(buf)),
			Date:    time.Now().Format("20060102"),
		}})
		if err != nil {
			log.Fatal(err)
		}
	}
}
