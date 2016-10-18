// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type SSHHostKeySerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&SSHHostKeySerializationSuite{})

func (s *SSHHostKeySerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "ssh-host-keys"
	s.sliceName = "ssh-host-keys"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importSSHHostKeys(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["ssh-host-keys"] = []interface{}{}
	}
}

func (s *SSHHostKeySerializationSuite) TestNewSSHHostKey(c *gc.C) {
	args := SSHHostKeyArgs{
		MachineID: "foo",
		Keys:      []string{"one", "two", "three"},
	}
	key := newSSHHostKey(args)
	c.Assert(key.MachineID(), gc.Equals, args.MachineID)
	c.Assert(key.Keys(), jc.DeepEquals, args.Keys)
}

func (s *SSHHostKeySerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := sshHostKeys{
		Version: 1,
		SSHHostKeys_: []*sshHostKey{
			newSSHHostKey(SSHHostKeyArgs{
				MachineID: "foo",
				Keys:      []string{"one", "two", "three"},
			}),
			newSSHHostKey(SSHHostKeyArgs{MachineID: "bar"}),
		},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	keys, err := importSSHHostKeys(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(keys, jc.DeepEquals, initial.SSHHostKeys_)
}
