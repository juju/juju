// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	goyaml "gopkg.in/yaml.v1"
)

// CacheFile represents the YAML structure of the file
// $JUJU_HOME/environments/cache.yaml
type CacheFile struct {
	// Server maps the name of the server to the server-uuid
	Server map[string]ServerUser `yaml:"server-user"`
	// ServerData is a map of server-uuid to the data for that server.
	ServerData map[string]ServerData `yaml:"server-data"`
	// Environment maps the local name of the environment to the details
	// for that environment
	Environment map[string]EnvironmentData `yaml:"environment"`
}

// ServerUser represents a user on a server, but not an environment on that
// server.  Used for server based commands like login, list and use.
type ServerUser struct {
	ServerUUID string `yaml:"server-uuid"`
	User       string `yaml:"user"`
}

// ServerData holds the end point details for the API servers running
// in the state server environment.
type ServerData struct {
	APIEndpoints    []string `yaml:"api-endpoints"`
	ServerHostnames []string `yaml:"server-hostnames,omitempty"`
	CACert          string   `yaml:"ca-cert"`
	// Identities is a mapping of full username to credentials.
	Identities      map[string]string      `yaml:"identities"`
	BootstrapConfig map[string]interface{} `yaml:"bootstrap-config,omitempty"`
}

// EnvironmentData represents a single environment running in a Juju
// Environment Server.
type EnvironmentData struct {
	User            string `yaml:"user"`
	EnvironmentUUID string `yaml:"env-uuid"`
	ServerUUID      string `yaml:"server-uuid"`
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
				Server:      make(map[string]ServerUser),
				ServerData:  make(map[string]ServerData),
				Environment: make(map[string]EnvironmentData),
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
	if envData, ok := cache.Environment[envName]; ok {
		srvData, ok = cache.ServerData[envData.ServerUUID]
		if !ok {
			return nil, errors.Errorf("missing server data for environment %q", envName)
		}
		info.user = envData.User
		info.environmentUUID = envData.EnvironmentUUID
		info.serverUUID = envData.ServerUUID
	} else {
		srvUser, ok := cache.Server[envName]
		if !ok {
			return nil, errors.NotFoundf("environment %q", envName)
		}
		srvData, ok = cache.ServerData[srvUser.ServerUUID]
		if !ok {
			return nil, errors.Errorf("missing server data for environment %q", envName)
		}
		info.user = srvUser.User
		info.serverUUID = srvUser.ServerUUID
	}

	info.credentials = srvData.Identities[info.user]
	info.caCert = srvData.CACert
	info.apiEndpoints = srvData.APIEndpoints
	info.apiHostnames = srvData.ServerHostnames
	if info.serverUUID == info.environmentUUID {
		info.bootstrapConfig = srvData.BootstrapConfig
	}
	return info, nil
}

func (cache *CacheFile) updateInfo(info *environInfo) error {
	// If the info is new, then check for name clashes.
	if info.source == sourceCreated {
		if _, found := cache.Environment[info.name]; found {
			return ErrEnvironInfoAlreadyExists
		}
		if server, found := cache.Server[info.name]; found {
			// Error if we are not trying to add
			// in the initial environment for this
			// server.
			if info.environmentUUID != server.ServerUUID {
				return ErrEnvironInfoAlreadyExists
			}
		}
	}

	// If the serverUUID and environmentUUID are the same, or the
	// environmentUUID is not specified, then add a name entry
	// under the server.
	serverUser := ServerUser{
		User:       info.user,
		ServerUUID: info.serverUUID,
	}
	if info.environmentUUID == "" || info.environmentUUID == info.serverUUID {
		cache.Server[info.name] = serverUser
	}

	// Only add environment entries if the environmentUUID was specified
	if info.environmentUUID != "" {
		cache.Environment[info.name] = EnvironmentData{
			User:            info.user,
			EnvironmentUUID: info.environmentUUID,
			ServerUUID:      info.serverUUID,
		}
	}

	// Check to see if the cache file has some info for the server already.
	serverData := cache.ServerData[info.serverUUID]
	serverData.APIEndpoints = info.apiEndpoints
	serverData.ServerHostnames = info.apiHostnames
	serverData.CACert = info.caCert
	if info.bootstrapConfig != nil {
		serverData.BootstrapConfig = info.bootstrapConfig
	}
	if serverData.Identities == nil {
		serverData.Identities = make(map[string]string)
	}
	serverData.Identities[info.user] = info.credentials
	cache.ServerData[info.serverUUID] = serverData
	return nil
}

func (cache *CacheFile) removeInfo(info *environInfo) error {
	if srvUser, srvFound := cache.Server[info.name]; srvFound {
		cache.cleanupAllServerReferences(srvUser.ServerUUID)
		return nil
	}
	envUser, envFound := cache.Environment[info.name]
	if !envFound {
		return errors.New("environment info has already been removed")
	}
	serverUUID := envUser.ServerUUID

	delete(cache.Environment, info.name)
	delete(cache.Server, info.name)
	// Look to see if there are any other environments using the serverUUID.
	// If there aren't, then we also clean up the server data, otherwise we
	// need to leave the server data there.
	for _, envUser := range cache.Environment {
		if envUser.ServerUUID == serverUUID {
			return nil
		}
	}
	delete(cache.ServerData, serverUUID)
	return nil
}

func (cache *CacheFile) cleanupAllServerReferences(serverUUID string) {
	// NOTE: it is safe in Go to remove elements from a map while iterating.
	for name, envUser := range cache.Environment {
		if envUser.ServerUUID == serverUUID {
			delete(cache.Environment, name)
		}
	}
	for name, srvUser := range cache.Server {
		if srvUser.ServerUUID == serverUUID {
			delete(cache.Server, name)
		}
	}
	delete(cache.ServerData, serverUUID)
}
