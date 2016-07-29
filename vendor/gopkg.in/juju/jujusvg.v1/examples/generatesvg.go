package main

// This is a demo application that uses the jujusvg library to build a bundle SVG
// from a given bundle.yaml file.

import (
	"io/ioutil"
	"log"
	"os"
	"strings"

	"gopkg.in/juju/charm.v6-unstable"

	// Import the jujusvg library and the juju charm library
	"gopkg.in/juju/jujusvg.v1"
)

// iconURL takes a reference to a charm and returns the URL for that charm's icon.
// In this case, we're using the api.jujucharms.com API to provide the icon's URL.
func iconURL(ref *charm.URL) string {
	return "https://api.jujucharms.com/charmstore/v4/" + ref.Path() + "/icon.svg"
}

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Please provide the name of a bundle file as the first argument")
	}

	// First, we need to read our bundle data into a []byte
	bundle_data, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		log.Fatalf("Error reading bundle: %s\n", err)
	}

	// Next, generate a charm.Bundle from the bytearray by passing it to ReadNewBundleData.
	// This gives us an in-memory object representation of the bundle that we can pass to jujusvg
	bundle, err := charm.ReadBundleData(strings.NewReader(string(bundle_data)))
	if err != nil {
		log.Fatalf("Error parsing bundle: %s\n", err)
	}

	fetcher := &jujusvg.HTTPFetcher{
		IconURL: iconURL,
	}
	// Next, build a canvas of the bundle.  This is a simplified version of a charm.Bundle
	// that contains just the position information and charm icon URLs necessary to build
	// the SVG representation of the bundle
	canvas, err := jujusvg.NewFromBundle(bundle, iconURL, fetcher)
	if err != nil {
		log.Fatalf("Error generating canvas: %s\n", err)
	}

	// Finally, marshal that canvas as SVG to os.Stdout; this will print the SVG data
	// required to generate an image of the bundle.
	canvas.Marshal(os.Stdout)
}
