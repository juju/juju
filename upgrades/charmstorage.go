// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/provider"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
)

var (
	charmBundleURL            = (*state.Charm).BundleURL
	charmStoragePath          = (*state.Charm).StoragePath
	stateAddCharmStoragePaths = state.AddCharmStoragePaths
)

// migrateCharmStorage copies uploaded charms from provider storage
// to environment storage, and then adds the storage path into the
// charm's document in state.
func migrateCharmStorage(st *state.State, agentConfig agent.Config) error {
	logger.Debugf("migrating charms to environment storage")
	charms, err := st.AllCharms()
	if err != nil {
		return err
	}
	storage := storage.NewStorage(st.EnvironUUID(), st.MongoSession())

	// Local and manual provider host storage on the state server's
	// filesystem, and serve via HTTP storage. The storage worker
	// doesn't run yet, so we just open the files directly.
	fetchCharmArchive := fetchCharmArchive
	providerType := agentConfig.Value(agent.ProviderType)
	if providerType == provider.Local || provider.IsManual(providerType) {
		storageDir := agentConfig.Value(agent.StorageDir)
		fetchCharmArchive = localstorage{storageDir}.fetchCharmArchive
	}

	storagePaths := make(map[*charm.URL]string)
	for _, ch := range charms {
		if ch.IsPlaceholder() {
			logger.Debugf("skipping %s, placeholder charm", ch.URL())
			continue
		}
		if !ch.IsUploaded() {
			logger.Debugf("skipping %s, not uploaded to provider storage", ch.URL())
			continue
		}
		if charmStoragePath(ch) != "" {
			logger.Debugf("skipping %s, already in environment storage", ch.URL())
			continue
		}
		url := charmBundleURL(ch)
		if url == nil {
			logger.Debugf("skipping %s, has no bundle URL", ch.URL())
			continue
		}
		uuid, err := utils.NewUUID()
		if err != nil {
			return err
		}
		data, err := fetchCharmArchive(url)
		if err != nil {
			return err
		}

		curl := ch.URL()
		storagePath := fmt.Sprintf("charms/%s-%s", curl, uuid)
		logger.Debugf("uploading %s to %q in environment storage", curl, storagePath)
		err = storage.Put(storagePath, bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return errors.Annotatef(err, "failed to upload %s to storage", curl)
		}
		storagePaths[curl] = storagePath
	}

	return stateAddCharmStoragePaths(st, storagePaths)
}

func fetchCharmArchive(url *url.URL) ([]byte, error) {
	client := utils.GetNonValidatingHTTPClient()
	resp, err := client.Get(url.String())
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get %q", url)
	}
	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot read charm archive")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("cannot get %q: %s %s", url, resp.Status, body)
	}
	return body, nil
}

type localstorage struct {
	storageDir string
}

func (s localstorage) fetchCharmArchive(url *url.URL) ([]byte, error) {
	path := filepath.Join(s.storageDir, url.Path)
	return ioutil.ReadFile(path)
}
