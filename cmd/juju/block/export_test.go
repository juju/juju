// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

var (
	BlockClient   = &getBlockClientAPI
	UnblockClient = &getUnblockClientAPI
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
	return nil
}
