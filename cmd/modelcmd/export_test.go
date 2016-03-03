// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

var (
	GetCurrentControllerFilePath = getCurrentControllerFilePath
	GetConfigStore               = &getConfigStore
	EndpointRefresher            = &endpointRefresher
)

// NewModelCommandBase returns a new ModelCommandBase with the model name, client,
// and error as specified for testing purposes.
// If getterErr != nil then the NewModelGetter returns the specified error.
func NewModelCommandBase(controller, model string, client ModelGetter, getterErr error) *ModelCommandBase {
	return &ModelCommandBase{
		controllerName:  controller,
		modelName:       model,
		envGetterClient: client,
		envGetterErr:    getterErr,
	}
}
