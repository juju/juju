// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	goyaml "gopkg.in/yaml.v2"
)

// CacheFile represents the YAML structure of the file
// $JUJU_DATA/controller-config.yaml
type CacheFile struct {
	// Server maps the name of the server to the server-uuid
	Server map[string]ServerUser `yaml:"server-user,omitempty"`
	// ServerData is a map of server-uuid to the data for that server.
	ServerData map[string]ServerData `yaml:"server-data,omitempty"`
}

// ServerUser represents a user on a server, but not an environment on that
// server.  Used for server based commands like login, list and use.
type ServerUser struct {
	ServerUUID string `yaml:"server-uuid"`
}

// ServerData holds the end point details for the API servers running
// in the controller environment.
type ServerData struct {
	BootstrapConfig map[string]interface{} `yaml:"bootstrap-config,omitempty"`
}

// All synchronisation locking is expected to be done outside the
// read and write methods.
func readCacheFile(filename string) (CacheFile, error) {
	data, err := ioutil.ReadFile(filename)
	var content CacheFile
	if err != nil {
		if os.IsNotExist(err) {
			// If the file doesn't exist, then we return an empty
			// CacheFile.
			return CacheFile{
				Server:     make(map[string]ServerUser),
				ServerData: make(map[string]ServerData),
			}, nil
		}
		return content, err
	}
	if err := goyaml.Unmarshal(data, &content); err != nil {
		return content, errors.Annotatef(err, "error unmarshalling %q", filename)
	}
	return content, nil
}

func writeCacheFile(filename string, content CacheFile) error {
	data, err := goyaml.Marshal(content)
	if err != nil {
		return errors.Annotate(err, "cannot marshal cache file")
	}
	err = ioutil.WriteFile(filename, data, 0600)
	return errors.Annotate(err, "cannot write file")
}

func (cache CacheFile) readInfo(envName string) (*environInfo, error) {
	info := &environInfo{
		name:   envName,
		source: sourceCache,
	}
	var srvData ServerData
	srvUser, ok := cache.Server[envName]
	if !ok {
		return nil, errors.NotFoundf("model %q", envName)
	}
	srvData, ok = cache.ServerData[srvUser.ServerUUID]
	if !ok {
		return nil, errors.Errorf("missing server data for model %q", envName)
	}
	info.serverUUID = srvUser.ServerUUID
	srvData, ok = cache.ServerData[srvUser.ServerUUID]
	if !ok {
		return nil, errors.Errorf("missing server data for model %q", envName)
	}

	info.bootstrapConfig = srvData.BootstrapConfig
	return info, nil
}

func (cache *CacheFile) updateInfo(info *environInfo) error {
	// If the info is new, then check for name clashes.
	if info.source == sourceCreated {
		if _, found := cache.Server[info.name]; found {
			return ErrEnvironInfoAlreadyExists
		}
	}

	serverUser := ServerUser{
		ServerUUID: info.serverUUID,
	}
	cache.Server[info.name] = serverUser

	// Check to see if the cache file has some info for the server already.
	serverData := cache.ServerData[info.serverUUID]
	if info.bootstrapConfig != nil {
		serverData.BootstrapConfig = info.bootstrapConfig
	}
	cache.ServerData[info.serverUUID] = serverData
	return nil
}

func (cache *CacheFile) removeInfo(info *environInfo) error {
	if srvUser, srvFound := cache.Server[info.name]; srvFound {
		cache.cleanupAllServerReferences(srvUser.ServerUUID)
		return nil
	}
	modelUser, envFound := cache.Server[info.name]
	if !envFound {
		return errors.New("model info has already been removed")
	}
	serverUUID := modelUser.ServerUUID

	delete(cache.Server, info.name)
	// Look to see if there are any other environments using the serverUUID.
	// If there aren't, then we also clean up the server data, otherwise we
	// need to leave the server data there.
	for _, modelUser := range cache.Server {
		if modelUser.ServerUUID == serverUUID {
			return nil
		}
	}
	delete(cache.ServerData, serverUUID)
	return nil
}

func (cache *CacheFile) cleanupAllServerReferences(serverUUID string) {
	// NOTE: it is safe in Go to remove elements from a map while iterating.
	for name, srvUser := range cache.Server {
		if srvUser.ServerUUID == serverUUID {
			delete(cache.Server, name)
		}
	}
	delete(cache.ServerData, serverUUID)
}
