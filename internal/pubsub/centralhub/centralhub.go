// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package centralhub

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/pubsub/v2"
	"github.com/juju/utils/v4"
	"gopkg.in/yaml.v2"

	internallogger "github.com/juju/juju/internal/logger"
	internalpubsub "github.com/juju/juju/internal/pubsub"
)

// New returns a new structured hub using yaml marshalling with an origin
// specified. The post processing ensures that the maps all have string keys
// so they messages can be marshalled between apiservers.
func New(origin names.Tag) *pubsub.StructuredHub {
	return pubsub.NewStructuredHub(
		&pubsub.StructuredHubConfig{
			Logger:     internalpubsub.WrapLogger(internallogger.GetLogger("juju.centralhub")),
			Marshaller: &yamlMarshaller{},
			Annotations: map[string]interface{}{
				"origin": origin.String(),
			},
			PostProcess: ensureStringMaps,
		})
}

type yamlMarshaller struct{}

// Marshal implements Marshaller.
func (*yamlMarshaller) Marshal(v interface{}) ([]byte, error) {
	return yaml.Marshal(v)
}

// Unmarshal implements Marshaller.
func (*yamlMarshaller) Unmarshal(data []byte, v interface{}) error {
	return yaml.Unmarshal(data, v)
}

func ensureStringMaps(in map[string]interface{}) (map[string]interface{}, error) {
	out, err := utils.ConformYAML(in)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return out.(map[string]interface{}), nil
}
