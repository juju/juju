// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// ReleaseFlags represents a way to have multiple values passed to the flags
type ReleaseFlags []string

// Set will append a config value to the config flags.
func (c *ReleaseFlags) Set(value string) error {
	*c = append(*c, value)
	return nil
}

func (c *ReleaseFlags) String() string {
	return strings.Join(*c, ",")
}

func main() {
	var rawReleases ReleaseFlags
	flag.Var(&rawReleases, "release", "A list of releases the streams support")
	flag.Parse()

	path := flag.Arg(0)

	releases := parseRelease(rawReleases, path)

	ip, err := externalIP()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(ip)

	// Serve streams as a json output.
	for k, release := range releases {
		fmt.Printf(" - Registering endpoint %s %v\n", k, release)
		key := fmt.Sprintf("/streams/v1/com.ubuntu.juju-%s-tools.json", k)
		http.HandleFunc(key, handleRelease(k, release))
	}
	http.HandleFunc("/streams/v1/index.json", handleIndex(releases))
	http.HandleFunc("/streams/v1/index2.json", handleIndex(releases))
	http.HandleFunc("/streams/v1/mirrors.json", handleMirror(releases))
	http.HandleFunc("/streams/v1/cpc-mirrors.json", handleCPCMirror(releases))

	// Serve all agent binaries as a static file server.
	agentPath := filepath.Join(path, "/tools/")
	fmt.Printf(" - Serving agent binaries %s\n", agentPath)
	fs := http.FileServer(http.Dir(agentPath))
	http.Handle("/", fs)

	if err := http.ListenAndServe(":8081", nil); err != nil {
		log.Fatal(err)
	}
}

func handleRelease(name string, release []Release) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		type Version struct {
			Size    int    `json:"size"`
			Path    string `json:"path"`
			SHA256  string `json:"sha256"`
			Version string `json:"version"`
		}
		type Versions struct {
			Items map[string]Version `json:"items"`
		}
		type Product struct {
			DataType string              `json:"datatype"`
			Format   string              `json:"format"`
			FType    string              `json:"ftype"`
			Release  string              `json:"release"`
			Arch     string              `json:"arch"`
			Versions map[string]Versions `json:"versions"`
		}

		now := time.Now().Format("20060102")
		products := make(map[string]Product)
		for _, r := range release {
			path := fmt.Sprintf("com.ubuntu.juju:%s:%s", r.Version, r.Arch)
			if p, ok := products[path]; ok {
				p.Versions[now].Items[fmt.Sprintf("%s-%s-%s", r.JujuVersion, r.Series, r.Arch)] = Version{
					Size:    r.Size,
					Path:    r.Path,
					SHA256:  r.SHA256,
					Version: r.JujuVersion,
				}
				continue
			}

			products[path] = Product{
				DataType: "content-download",
				Format:   "products:1.0",
				FType:    "tar.gz",
				Release:  r.Series,
				Arch:     r.Arch,
				Versions: map[string]Versions{
					now: Versions{
						Items: map[string]Version{
							fmt.Sprintf("%s-%s-%s", r.JujuVersion, r.Series, r.Arch): Version{
								Size:    r.Size,
								Path:    r.Path,
								SHA256:  r.SHA256,
								Version: r.JujuVersion,
							},
						},
					},
				},
			}
		}

		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "\t")
		encoder.Encode(struct {
			ContentID string             `json:"content_id"`
			DataType  string             `json:"datatype"`
			Format    string             `json:"format"`
			Product   map[string]Product `json:"products"`
		}{
			ContentID: fmt.Sprintf("com.ubuntu.juju:%s:tools", name),
			DataType:  "content-download",
			Format:    "products:1.0",
			Product:   products,
		})
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

		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "\t")
		encoder.Encode(struct {
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
		type Mirror struct {
			DataType string `json:"datatype"`
			Path     string `json:"path"`
			Format   string `json:"format"`
		}
		mirrors := make(map[string][]Mirror)
		for k := range releases {
			mirrors[fmt.Sprintf("com.ubuntu.juju:%s:tools", k)] = []Mirror{{
				DataType: "content-download",
				Path:     "streams/v1/cpc-mirrors.json",
				Format:   "mirrors:1.0",
			}}
		}

		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "\t")
		encoder.Encode(struct {
			Mirrors map[string][]Mirror `json:"mirrors"`
		}{
			Mirrors: mirrors,
		})
	}
}

func handleCPCMirror(releases map[string][]Release) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		mirrors := make(map[string][]string)
		for k := range releases {
			mirrors[fmt.Sprintf("com.ubuntu.juju:%s:tools", k)] = make([]string, 0)
		}

		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "\t")
		encoder.Encode(struct {
			Format  string              `json:"format"`
			Mirrors map[string][]string `json:"mirrors"`
		}{
			Format:  "index:1.0",
			Mirrors: mirrors,
		})
	}
}

type Release struct {
	JujuVersion string
	Series      string
	Version     string
	Arch        string
	Path        string
	Size        int
	SHA256      string
}

func parseRelease(r ReleaseFlags, path string) map[string][]Release {
	results := make(map[string][]Release)
	for _, v := range r {
		parts := strings.Split(v, ",")
		stream := parts[0]

		// Read in the binary to get the size and the sha256
		buf, err := ioutil.ReadFile(filepath.Join(path, "tools", parts[3]))
		if err != nil {
			log.Fatal(err)
		}

		versions := strings.Split(parts[1], "-")
		release := Release{
			JujuVersion: parts[2],
			Series:      versions[0],
			Version:     versions[1],
			Arch:        versions[2],
			Path:        parts[3],
			Size:        len(buf),
			SHA256:      fmt.Sprintf("%x", sha256.Sum256(buf)),
		}
		results[stream] = append(results[stream], release)
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
