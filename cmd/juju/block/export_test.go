// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import "github.com/juju/juju/apiserver/params"

var (
	BlockClient   = &getBlockClientAPI
	UnblockClient = &getUnblockClientAPI
	ListClient    = &getBlockListAPI
)

type MockBlockClient struct {
	BlockType string
	Msg       string
}

func (c *MockBlockClient) Close() error {
	return nil
}

func (c *MockBlockClient) SwitchBlockOn(blockType, msg string) error {
	c.BlockType = blockType
	c.Msg = msg
	return nil
}

func (c *MockBlockClient) SwitchBlockOff(blockType string) error {
	c.BlockType = blockType
	c.Msg = ""
	return nil
}

func (c *MockBlockClient) List() ([]params.Block, error) {
	if c.BlockType == "" {
		return []params.Block{}, nil
	}

	return []params.Block{
		params.Block{
			Type:    c.BlockType,
			Message: c.Msg,
		},
	}, nil
}
