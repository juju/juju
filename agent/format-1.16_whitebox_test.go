// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The format tests are white box tests, meaning that the tests are in the
// same package as the code, as all the format details are internal to the
// package.

package agent

import (
	"io/ioutil"
	"path/filepath"

	gc "launchpad.net/gocheck"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

type format_1_16Suite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&format_1_16Suite{})

func (s *format_1_16Suite) TestMissingUpgradedToVersion(c *gc.C) {
	dataDir := c.MkDir()
	formatPath := filepath.Join(dataDir, legacyFormatFilename)
	err := utils.AtomicWriteFile(formatPath, []byte(legacyFormatFileContents), 0600)
	c.Assert(err, gc.IsNil)
	configPath := filepath.Join(dataDir, agentConfigFilename)
	err = utils.AtomicWriteFile(configPath, []byte(configDataWithoutUpgradedToVersion), 0600)
	c.Assert(err, gc.IsNil)
	readConfig, err := ReadConf(configPath)
	c.Assert(err, gc.IsNil)
	c.Assert(readConfig.UpgradedToVersion(), gc.Equals, version.MustParse("1.16.0"))
}

func (*format_1_16Suite) TestReadConfReadsLegacyFormatAndWritesNew(c *gc.C) {
	dataDir := c.MkDir()
	formatPath := filepath.Join(dataDir, legacyFormatFilename)
	err := utils.AtomicWriteFile(formatPath, []byte(legacyFormatFileContents), 0600)
	c.Assert(err, gc.IsNil)
	configPath := filepath.Join(dataDir, agentConfigFilename)
	err = utils.AtomicWriteFile(configPath, []byte(agentConfig1_16Contents), 0600)
	c.Assert(err, gc.IsNil)

	config, err := ReadConf(configPath)
	c.Assert(err, gc.IsNil)
	c.Assert(config, gc.NotNil)
	// Test we wrote a currently valid config.
	config, err = ReadConf(configPath)
	c.Assert(err, gc.IsNil)
	c.Assert(config, gc.NotNil)
	c.Assert(config.UpgradedToVersion(), jc.DeepEquals, version.MustParse("1.16.0"))
	c.Assert(config.Jobs(), gc.HasLen, 0)

	// Old format was deleted.
	assertFileNotExist(c, formatPath)
	// And new contents were written.
	data, err := ioutil.ReadFile(configPath)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Not(gc.Equals), agentConfig1_16Contents)
}

const legacyFormatFileContents = "format 1.16"

var agentConfig1_16Contents = `
tag: machine-0
nonce: user-admin:bootstrap
cacert: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNXekNDQWNhZ0F3SUJBZ0lCQURBTEJna3Foa2lHOXcwQkFRVXdRekVOTUFzR0ExVUVDaE1FYW5WcWRURXkKTURBR0ExVUVBd3dwYW5WcWRTMW5aVzVsY21GMFpXUWdRMEVnWm05eUlHVnVkbWx5YjI1dFpXNTBJQ0pzYjJOaApiQ0l3SGhjTk1UUXdNekExTVRJeE9ERTJXaGNOTWpRd016QTFNVEl5TXpFMldqQkRNUTB3Q3dZRFZRUUtFd1JxCmRXcDFNVEl3TUFZRFZRUUREQ2xxZFdwMUxXZGxibVZ5WVhSbFpDQkRRU0JtYjNJZ1pXNTJhWEp2Ym0xbGJuUWcKSW14dlkyRnNJakNCbnpBTkJna3Foa2lHOXcwQkFRRUZBQU9CalFBd2dZa0NnWUVBd3NaVUg3NUZGSW1QUWVGSgpaVnVYcmlUWmNYdlNQMnk0VDJaSU5WNlVrY2E5VFdXb01XaWlPYm4yNk03MjNGQllPczh3WHRYNEUxZ2l1amxYCmZGeHNFckloczEyVXQ1S3JOVkkyMlEydCtVOGViakZMUHJiUE5Fb3pzdnU3UzFjZklFbjBXTVg4MWRBaENOMnQKVkxGaC9hS3NqSHdDLzJ5Y3Z0VSttTngyVG5FQ0F3RUFBYU5qTUdFd0RnWURWUjBQQVFIL0JBUURBZ0NrTUE4RwpBMVVkRXdFQi93UUZNQU1CQWY4d0hRWURWUjBPQkJZRUZKVUxKZVlIbERsdlJ3T0owcWdyemcwclZGZUVNQjhHCkExVWRJd1FZTUJhQUZKVUxKZVlIbERsdlJ3T0owcWdyemcwclZGZUVNQXNHQ1NxR1NJYjNEUUVCQlFPQmdRQ2UKRlRZbThsWkVYZUp1cEdPc3pwc2pjaHNSMEFxeXROZ1dxQmE1cWUyMS9xS2R3TUFSQkNFMTU3eUxGVnl6MVoycQp2YVhVNy9VKzdKbGNiWmtHRHJ5djE2S2UwK2RIY3NEdG5jR2FOVkZKMTAxYnNJNG1sVEkzQWpQNDErNG5mQ0VlCmhwalRvYm1YdlBhOFN1NGhQYTBFc1E4bXFaZGFabmdwRU0vb1JiZ0RMdz09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K
stateaddresses:
- localhost:37017
statepassword: OlUMkte5J3Ss0CH9yxedilIC
apiaddresses:
- localhost:17070
apipassword: OlUMkte5J3Ss0CH9yxedilIC
oldpassword: oBlMbFUGvCb2PMFgYVzjS6GD
values:
  PROVIDER_TYPE: local
  SHARED_STORAGE_ADDR: 10.0.3.1:8041
  SHARED_STORAGE_DIR: /home/user/.juju/local/shared-storage
  STORAGE_ADDR: 10.0.3.1:8040
  STORAGE_DIR: /home/user/.juju/local/storage
stateservercert: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNJakNDQVkyZ0F3SUJBZ0lCQURBTEJna3Foa2lHOXcwQkFRVXdRekVOTUFzR0ExVUVDaE1FYW5WcWRURXkKTURBR0ExVUVBd3dwYW5WcWRTMW5aVzVsY21GMFpXUWdRMEVnWm05eUlHVnVkbWx5YjI1dFpXNTBJQ0pzYjJOaApiQ0l3SGhjTk1UUXdNekExTVRJeE9ESXlXaGNOTWpRd016QTFNVEl5TXpJeVdqQWJNUTB3Q3dZRFZRUUtFd1JxCmRXcDFNUW93Q0FZRFZRUURFd0VxTUlHZk1BMEdDU3FHU0liM0RRRUJBUVVBQTRHTkFEQ0JpUUtCZ1FDdVA0dTAKQjZtbGs0V0g3SHFvOXhkSFp4TWtCUVRqV2VLTkhERzFMb21SWmc2RHA4Z0VQK0ZNVm5IaUprZW1pQnJNSEk3OAo5bG4zSVRBT0NJT0xna0NkN3ZsaDJub2FheTlSeXpUaG9PZ0RMSzVpR0VidmZDeEFWZThhWDQvbThhOGNLWE9TCmJJZTZFNnVtb0wza0JNaEdiL1QrYW1xbHRjaHVNRXJhanJSVit3SURBUUFCbzFJd1VEQU9CZ05WSFE4QkFmOEUKQkFNQ0FCQXdIUVlEVlIwT0JCWUVGRTV1RFg3UlRjckF2ajFNcWpiU2w1M21pR0NITUI4R0ExVWRJd1FZTUJhQQpGSlVMSmVZSGxEbHZSd09KMHFncnpnMHJWRmVFTUFzR0NTcUdTSWIzRFFFQkJRT0JnUUJUNC8vZkpESUcxM2dxClBiamNnUTN6eHh6TG12STY5Ty8zMFFDbmIrUGZObDRET0U1SktwVE5OTjhkOEJEQWZPYStvWE5neEM3VTZXdjUKZjBYNzEyRnlNdUc3VXJEVkNDY0kxS3JSQ0F0THlPWUREL0ZPblBwSWdVQjF1bFRnOGlRUzdlTjM2d0NEL21wVApsUVVUS2FuU00yMnhnWWJKazlRY1dBSzQ0ZjA4SEE9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==
stateserverkey: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlDV3dJQkFBS0JnUUN1UDR1MEI2bWxrNFdIN0hxbzl4ZEhaeE1rQlFUaldlS05IREcxTG9tUlpnNkRwOGdFClArRk1WbkhpSmtlbWlCck1ISTc4OWxuM0lUQU9DSU9MZ2tDZDd2bGgybm9hYXk5Unl6VGhvT2dETEs1aUdFYnYKZkN4QVZlOGFYNC9tOGE4Y0tYT1NiSWU2RTZ1bW9MM2tCTWhHYi9UK2FtcWx0Y2h1TUVyYWpyUlYrd0lEQVFBQgpBb0dBRERJZ2FoSmJPbDZQNndxUEwwSlVHOGhJRzY1S1FFdHJRdXNsUTRRbFZzcm8yeWdrSkwvLzJlTDNCNWdjClRiaWEvNHhFS2Nwb1U1YThFVTloUGFONU9EYnlkVEsxQ1I3R2JXSGkwWm1LbGZCUlR4bUpxakdKVU1CSmI4a0QKNStpMzlvcXdQS3dnaXoyTVR5SHZKZFFJVHB0ZDVrbEQyYjU1by9YWFRCTnk2NGtDUVFEbXRFWHNTL2kxTm5pSwozZVJkeHM4UVFGN0pKVG5SR042ZUh6ZHlXb242Zjl2ZkxrSDROWUdxcFUydjVBNUl1Nno3K3NJdXVHU2ZSeEI1CktrZVFXdlVQQWtFQXdWcVdlczdmc3NLbUFCZGxER3ozYzNxMjI2eVVaUE00R3lTb1cxYXZsYzJ1VDVYRm9vVUsKNjRpUjJuU2I1OHZ2bGY1RjRRMnJuRjh2cFRLcFJwK0lWUUpBTlcwZ0dFWEx0ZU9FYk54UUMydUQva1o1N09rRApCNnBUdTVpTkZaMWtBSy9sY2p6YktDanorMW5Hc09vR2FNK1ZrdEVTY1JGZ3RBWVlDWWRDQldzYS93SkFROWJXCnlVdmdMTVlpbkJHWlFKelN6VStXN01oR1lJejllSGlLSVZIdTFTNlBKQmsyZUdrWmhiNHEvbXkvYnJxYzJ4R1YKenZxTzVaUjRFUXdQWEZvSTZRSkFkeVdDMllOTTF2a1BuWnJqZzNQQXFHRHJQMHJwNEZ0bFV4alh0ay8vcW9hNgpRcXVYcE9vNjd4THRieW1PTlJTdDFiZGE5ZE5tbGljMFVNZ0JQRHgrYnc9PQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQo=
apiport: 17070
`[1:]

const configDataWithoutUpgradedToVersion = `
tag: omg
nonce: a nonce
cacert: Y2EgY2VydA==
stateaddresses:
- localhost:1234
apiaddresses:
- localhost:1235
oldpassword: sekrit
values: {}
`
