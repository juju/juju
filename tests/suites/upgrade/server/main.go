// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// release=released:focal-amd64
// release=devel:focal-20.04-amd64

// ReleaseFlags represents a way to have multiple values passed to the flags
type ReleaseFlags []string

// Set will append a config value to the config flags.
func (c *ReleaseFlags) Set(value string) error {
	if !strings.Contains(value, ":") {
		return errors.Errorf("bad release pair, expected `key=value` for: %q", value)
	}
	*c = append(*c, value)
	return nil
}

func (c *ReleaseFlags) String() string {
	return strings.Join(*c, ",")
}

func main() {
	path := os.Args[1]

	var rawReleases ReleaseFlags
	flag.Var(&rawReleases, "release", "A list of releases the streams support")
	flag.Parse()

	releases := parseRelease(rawReleases)

	ip, err := externalIP()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(ip)

	// Serve streams as a json output.
	http.HandleFunc("/streams/v1/index.json", handleIndex(releases))
	http.HandleFunc("/streams/v1/index2.json", handleIndex(releases))
	http.HandleFunc("/streams/v1/cpc-mirrors.json", handleMirror(releases))

	// Serve all agent binaries as a static file server.
	fs := http.FileServer(http.Dir(filepath.Join(path, "/tools/agent/")))
	http.Handle("/agent/", fs)

	if err := http.ListenAndServe(":8081", nil); err != nil {
		log.Fatal(err)
	}
}

func handleIndex(releases map[string][]Release) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		type Index struct {
			DataType string   `json:"datatype"`
			Format   string   `json:"format"`
			Path     string   `json:"path"`
			Products []string `json:"products"`
		}

		indexes := make(map[string]Index)
		for k, r := range releases {
			var products []string
			for _, release := range r {
				products = append(products, fmt.Sprintf("com.ubuntu.juju:%s:%s", release.Version, release.Arch))
			}

			indexes[fmt.Sprintf("com.ubuntu.juju:%s:tools", k)] = Index{
				DataType: "content-download",
				Format:   "products:1.0",
				Path:     fmt.Sprintf("streams/v1/com.ubuntu.juju-%s-tools.json", k),
				Products: products,
			}
		}

		json.NewEncoder(w).Encode(struct {
			Format string           `json:"format"`
			Index  map[string]Index `json:"index"`
		}{
			Format: "index:1.0",
			Index:  indexes,
		})
	}
}

func handleMirror(releases map[string][]Release) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		mirrors := make(map[string][]string)
		for k := range releases {
			mirrors[fmt.Sprintf("com.ubuntu.juju:%s:tools", k)] = make([]string, 0)
		}

		json.NewEncoder(w).Encode(struct {
			Format string              `json:"format"`
			Index  map[string][]string `json:"mirrors"`
		}{
			Format: "index:1.0",
			Index:  mirrors,
		})
	}
}

type Release struct {
	Series  string
	Version string
	Arch    string
}

func parseRelease(r ReleaseFlags) map[string][]Release {
	results := make(map[string][]Release)
	for _, v := range r {
		parts := strings.Split(v, ":")
		stream := parts[0]

		versions := strings.Split(strings.Join(parts[1:], ":"), "-")
		release := Release{
			Series:  versions[0],
			Version: versions[1],
			Arch:    versions[2],
		}
		if _, ok := results[stream]; ok {
			results[stream] = append(results[stream], release)
		}
		results[stream] = []Release{release}
	}
	return results
}

func externalIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue // not an ipv4 address
			}
			return ip.String(), nil
		}
	}
	return "", errors.New("are you connected to the network?")
}
