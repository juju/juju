// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/altoros/gosigma/data"
)

func (c Client) getServers(rqspec RequestSpec) ([]data.Server, error) {
	u := c.endpoint + "servers"
	if rqspec == RequestDetail {
		u += "/detail"
	}

	r, err := c.https.Get(u, url.Values{"limit": {"0"}})
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	if err := r.VerifyJSON(200); err != nil {
		return nil, NewError(r, err)
	}

	return data.ReadServers(r.Body)
}

func (c Client) getServer(uuid string) (*data.Server, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, errEmptyUUID
	}

	u := c.endpoint + "servers/" + uuid + "/"

	r, err := c.https.Get(u, nil)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	if err := r.VerifyJSON(200); err != nil {
		return nil, NewError(r, err)
	}

	return data.ReadServer(r.Body)
}

func (c Client) startServer(uuid string, avoid []string) error {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return errEmptyUUID
	}

	u := c.endpoint + "servers/" + uuid + "/action/"

	var qq = make(url.Values)
	qq["do"] = []string{"start"}

	if len(avoid) > 0 {
		qq["avoid"] = []string{strings.Join(avoid, ",")}
	}

	r, err := c.https.Post(u, qq, nil)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if err := r.VerifyJSON(202); err != nil {
		return NewError(r, err)
	}

	return nil
}

func (c Client) stopServer(uuid string) error {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return errEmptyUUID
	}

	u := c.endpoint + "servers/" + uuid + "/action/"

	var qq = make(url.Values)
	qq["do"] = []string{"stop"}

	r, err := c.https.Post(u, qq, nil)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if err := r.VerifyJSON(202); err != nil {
		return NewError(r, err)
	}

	return nil
}

func (c Client) removeServer(uuid, recurse string) error {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return errEmptyUUID
	}

	u := c.endpoint + "servers/" + uuid + "/"

	var qq url.Values
	recurse = strings.TrimSpace(recurse)
	if recurse != "" {
		qq = make(url.Values)
		qq["recurse"] = []string{recurse}
	}

	r, err := c.https.Delete(u, qq, nil)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if err := r.VerifyCode(204); err != nil {
		return NewError(r, err)
	}

	return nil
}

func (c Client) createServer(components Components) ([]data.Server, error) {
	// serialize
	rr, err := components.marshal()
	if err != nil {
		return nil, err
	}

	// run request
	u := c.endpoint + "servers/"
	r, err := c.https.Post(u, nil, rr)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	if err := r.VerifyJSON(201); err != nil {
		return nil, NewError(r, err)
	}

	return data.ReadServers(r.Body)
}

func (c Client) getDrives(rqspec RequestSpec, libspec LibrarySpec) ([]data.Drive, error) {
	u := c.endpoint
	if libspec == LibraryMedia {
		u += "libdrives"
	} else {
		u += "drives"
	}
	if rqspec == RequestDetail {
		u += "/detail"
	}

	r, err := c.https.Get(u, url.Values{"limit": {"0"}})
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	if err := r.VerifyJSON(200); err != nil {
		return nil, NewError(r, err)
	}

	return data.ReadDrives(r.Body)
}

func (c Client) getDrive(uuid string, libspec LibrarySpec) (*data.Drive, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, errEmptyUUID
	}

	u := c.endpoint
	if libspec == LibraryMedia {
		u += "libdrives/"
	} else {
		u += "drives/"
	}
	u += uuid + "/"

	r, err := c.https.Get(u, nil)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	if err := r.VerifyJSON(200); err != nil {
		return nil, NewError(r, err)
	}

	return data.ReadDrive(r.Body)
}

func (c Client) cloneDrive(uuid string, libspec LibrarySpec, params CloneParams, avoid []string) (*data.Drive, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, errEmptyUUID
	}

	u := c.endpoint
	if libspec == LibraryMedia {
		u += "libdrives/"
	} else {
		u += "drives/"
	}
	u += uuid + "/action/"

	var qq = make(url.Values)
	qq["do"] = []string{"clone"}

	if len(avoid) > 0 {
		qq["avoid"] = []string{strings.Join(avoid, ",")}
	}

	rr, err := params.makeJSONReader()
	if err != nil {
		return nil, err
	}

	r, err := c.https.Post(u, qq, rr)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	if err := r.VerifyJSON(202); err != nil {
		return nil, NewError(r, err)
	}

	objs, err := data.ReadDrives(r.Body)

	if err != nil {
		return nil, err
	}

	if len(objs) == 0 {
		return nil, errors.New("no object was returned from server")
	}

	if libspec == LibraryMedia {
		// fix CloudSigma API result, disk has URI pointed to libdrive - must be drives
		uuid := objs[0].Resource.UUID
		objs[0].Resource = *data.MakeDriveResource(uuid)
	}

	return &objs[0], nil
}

func (c *Client) removeDrive(uuid string, libspec LibrarySpec) error {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return errEmptyUUID
	}

	u := c.endpoint
	if libspec == LibraryMedia {
		u += "libdrives/"
	} else {
		u += "drives/"
	}
	u += uuid + "/"

	r, err := c.https.Delete(u, nil, nil)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if err := r.VerifyCode(204); err != nil {
		return NewError(r, err)
	}

	return nil
}

func (c Client) getJob(uuid string) (*data.Job, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, errEmptyUUID
	}

	u := c.endpoint + "jobs/" + uuid + "/"

	r, err := c.https.Get(u, nil)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	if err := r.VerifyJSON(200); err != nil {
		return nil, NewError(r, err)
	}

	return data.ReadJob(r.Body)
}

func (c Client) readContext() (*data.Context, error) {

	const (
		DEVICE  = "/dev/ttyS1"
		REQUEST = "<\n\n>"
		EOT     = '\x04'
	)

	logger := c.logger

	// open server ctx device
	f, err := os.OpenFile(DEVICE, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("error OpenFile: %s", err)
	}
	defer f.Close()

	// schedule timeout, if defined
	readWriteTimeout := c.GetReadWriteTimeout()
	if readWriteTimeout > 0 {
		timer := time.AfterFunc(readWriteTimeout, func() {
			f.Close()
		})
		defer timer.Stop()
	}

	// writing request to service
	n, err := f.WriteString(REQUEST)
	if err != nil {
		return nil, fmt.Errorf("error WriteString: %s", err)
	}

	// check the request was written
	if n != len(REQUEST) {
		return nil, fmt.Errorf("invalid write length %d, wants %d", n, len(REQUEST))
	}

	// prepare buffered I/O object
	r := bufio.NewReader(f)

	// read until End-Of-Transfer (EOT) symbol or EOF
	bb, err := r.ReadBytes(EOT)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("error ReadBytes: %s", err)
	}

	// if EOT was read, truncate it
	if len(bb) > 0 {
		if last := len(bb) - 1; bb[last] == EOT {
			bb = bb[:last]
		}
	}

	// log server context as raw content
	if logger != nil {
		logger.Logf("")
		logger.Logf("server context:\n%s", string(bb))
		logger.Logf("")
	}

	// prepare reader around raw content
	rr := bytes.NewReader(bb)

	// parse server context JSON to the data.Context object
	return data.ReadContext(rr)
}

func (c Client) resizeDrive(obj data.Drive, newSize uint64) (*data.Drive, error) {

	// prepare endpoint URL
	u := c.endpoint + "drives/" + obj.UUID + "/action/"

	// prepare request query params
	var qq = make(url.Values)
	qq["do"] = []string{"resize"}

	// prepare request body
	obj.Size = newSize
	rr, err := data.WriteDrive(&obj)
	if err != nil {
		return nil, err
	}

	// do request
	r, err := c.https.Post(u, qq, rr)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	// verify reply
	if err := r.VerifyJSON(202); err != nil {
		return nil, NewError(r, err)
	}

	// read and parse reply body
	objs, err := data.ReadDrives(r.Body)

	if err != nil {
		return nil, err
	}

	// verify reply body content
	if len(objs) == 0 {
		return nil, errors.New("no object was returned from server")
	}

	// return result
	return &objs[0], nil
}
