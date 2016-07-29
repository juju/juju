// Copyright 2012-2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

/*
This is an example on how the Go library gomaasapi can be used to interact with
a real MAAS server.
Note that this is a provided only as an example and that real code should probably do something more sensible with errors than ignoring them or panicking.
*/
package main

import (
	"bytes"
	"fmt"
	"net/url"

	"github.com/juju/gomaasapi"
)

var apiKey string
var apiURL string
var apiVersion string

func getParams() {
	fmt.Println("Warning: this will create a node on the MAAS server; it should be deleted at the end of the run but if something goes wrong, that test node might be left over.  You've been warned.")
	fmt.Print("Enter API key: ")
	_, err := fmt.Scanf("%s", &apiKey)
	if err != nil {
		panic(err)
	}
	fmt.Print("Enter API URL: ")
	_, err = fmt.Scanf("%s", &apiURL)
	if err != nil {
		panic(err)
	}

	fmt.Print("Enter API version: ")
	_, err = fmt.Scanf("%s", &apiVersion)
	if err != nil {
		panic(err)
	}
}

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	getParams()

	// Create API server endpoint.
	authClient, err := gomaasapi.NewAuthenticatedClient(apiURL, apiKey, apiVersion)
	checkError(err)
	maas := gomaasapi.NewMAAS(*authClient)

	// Exercise the API.
	ManipulateNodes(maas)
	ManipulateFiles(maas)

	fmt.Println("All done.")
}

// ManipulateFiles exercises the /api/1.0/files/ API endpoint.  Most precisely,
// it uploads a files and then fetches it, making sure the received content
// is the same as the one that was sent.
func ManipulateFiles(maas *gomaasapi.MAASObject) {
	files := maas.GetSubObject("files")
	fileContent := []byte("test file content")
	fileName := "filename"
	filesToUpload := map[string][]byte{"file": fileContent}

	// Upload a file.
	fmt.Println("Uploading a file...")
	_, err := files.CallPostFiles("add", url.Values{"filename": {fileName}}, filesToUpload)
	checkError(err)
	fmt.Println("File sent.")

	// Fetch the file.
	fmt.Println("Fetching the file...")
	fileResult, err := files.CallGet("get", url.Values{"filename": {fileName}})
	checkError(err)
	receivedFileContent, err := fileResult.GetBytes()
	checkError(err)
	if bytes.Compare(receivedFileContent, fileContent) != 0 {
		panic("Received content differs from the content sent!")
	}
	fmt.Println("Got file.")

	// Fetch list of files.
	listFiles, err := files.CallGet("list", url.Values{})
	checkError(err)
	listFilesArray, err := listFiles.GetArray()
	checkError(err)
	fmt.Printf("We've got %v file(s)\n", len(listFilesArray))

	// Delete the file.
	fmt.Println("Deleting the file...")
	fileObject, err := listFilesArray[0].GetMAASObject()
	checkError(err)
	errDelete := fileObject.Delete()
	checkError(errDelete)

	// Count the files.
	listFiles, err = files.CallGet("list", url.Values{})
	checkError(err)
	listFilesArray, err = listFiles.GetArray()
	checkError(err)
	fmt.Printf("We've got %v file(s)\n", len(listFilesArray))
}

// ManipulateFiles exercises the /api/1.0/nodes/ API endpoint.  Most precisely,
// it lists the existing nodes, creates a new node, updates it and then
// deletes it.
func ManipulateNodes(maas *gomaasapi.MAASObject) {
	nodeListing := maas.GetSubObject("nodes")

	// List nodes.
	fmt.Println("Fetching list of nodes...")
	listNodeObjects, err := nodeListing.CallGet("list", url.Values{})
	checkError(err)
	listNodes, err := listNodeObjects.GetArray()
	checkError(err)
	fmt.Printf("Got list of %v nodes\n", len(listNodes))
	for index, nodeObj := range listNodes {
		node, err := nodeObj.GetMAASObject()
		checkError(err)
		hostname, err := node.GetField("hostname")
		checkError(err)
		fmt.Printf("Node #%d is named '%v' (%v)\n", index, hostname, node.URL())
	}

	// Create a node.
	fmt.Println("Creating a new node...")
	params := url.Values{"architecture": {"i386/generic"}, "mac_addresses": {"AA:BB:CC:DD:EE:FF"}}
	newNodeObj, err := nodeListing.CallPost("new", params)
	checkError(err)
	newNode, err := newNodeObj.GetMAASObject()
	checkError(err)
	newNodeName, err := newNode.GetField("hostname")
	checkError(err)
	fmt.Printf("New node created: %s (%s)\n", newNodeName, newNode.URL())

	// Update the new node.
	fmt.Println("Updating the new node...")
	updateParams := url.Values{"hostname": {"mynewname"}}
	newNodeObj2, err := newNode.Update(updateParams)
	checkError(err)
	newNodeName2, err := newNodeObj2.GetField("hostname")
	checkError(err)
	fmt.Printf("New node updated, now named: %s\n", newNodeName2)

	// Count the nodes.
	listNodeObjects2, err := nodeListing.CallGet("list", url.Values{})
	checkError(err)
	listNodes2, err := listNodeObjects2.GetArray()
	checkError(err)
	fmt.Printf("We've got %v nodes\n", len(listNodes2))

	// Delete the new node.
	fmt.Println("Deleting the new node...")
	errDelete := newNode.Delete()
	checkError(errDelete)

	// Count the nodes.
	listNodeObjects3, err := nodeListing.CallGet("list", url.Values{})
	checkError(err)
	listNodes3, err := listNodeObjects3.GetArray()
	checkError(err)
	fmt.Printf("We've got %v nodes\n", len(listNodes3))
}
