// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

type SSHKey struct {
	Key         string
	Fingerprint string
}

var (
	ValidKeyOne = SSHKey{
		`ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDEX/dPu4PmtvgK3La9zioCEDrJ` +
			`yUr6xEIK7Pr+rLgydcqWTU/kt7w7gKjOw4vvzgHfjKl09CWyvgb+y5dCiTk` +
			`9MxI+erGNhs3pwaoS+EavAbawB7iEqYyTep3YaJK+4RJ4OX7ZlXMAIMrTL+` +
			`UVrK89t56hCkFYaAgo3VY+z6rb/b3bDBYtE1Y2tS7C3au73aDgeb9psIrSV` +
			`86ucKBTl5X62FnYiyGd++xCnLB6uLximM5OKXfLzJQNS/QyZyk12g3D8y69` +
			`Xw1GzCSKX1u1+MQboyf0HJcG2ryUCLHdcDVppApyHx2OLq53hlkQ/yxdflD` +
			`qCqAE4j+doagSsIfC1T2T`,
		"86:ed:1b:cd:26:a0:a3:4c:27:35:49:60:95:b7:0f:68",
	}

	ValidKeyTwo = SSHKey{
		`ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDNC6zK8UMazlVgp8en8N7m7H/Y6` +
			`DoMWbmPFjXYRXu6iQJJ18hCtsfMe63E5/PBaOjDT8am0Sx3Eqn4ZzpWMj+z` +
			`knTcSd8xnMHYYxH2HStRWC1akTe4tTno2u2mqzjKd8f62URPtIocYCNRBls` +
			`9yjnq9SogI5EXgcx6taQcrIFcIK0SlthxxcMVSlLpnbReujW65JHtiMqoYA` +
			`OIALyO+Rkmtvb/ObmViDnwCKCN1up/xWt6J10MrAUtpI5b4prqG7FOqVMM/` +
			`zdgrVg6rUghnzdYeQ8QMyEv4mVSLzX0XIPcxorkl9q06s5mZmAzysEbKZCO` +
			`aXcLeNlXx/nkmuWslYCJ`,
		"2f:fb:b0:65:68:c8:4e:a6:1b:a6:4b:8d:14:0b:40:79",
	}

	ValidKeyThree = SSHKey{
		`ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCpGj1JMjGjAFt5wjARbIORyjQ/c` +
			`ZAiDyDHe/w8qmLKUG2KTs6586QqqM6DKPZiYesrzXqvZsWYV4B6OjLM1sxq` +
			`WjeDIl56PSnJ0+KP8pUV9KTkkKtRXxAoNg/II4l69e05qGffj9AcQ/7JPxx` +
			`eL14Ulvh/a69r3uVkw1UGVk9Bwm4eCOSCqKalYLA1k5da6crEAXn9hiXLGs` +
			`S9dOn3Lsqj5tK31aaUncue+a3iKb7R5LRFflDizzNS+h8tPuANQflOjOhR0` +
			`Vas0BsurgISseZZ0NIMISyWhZpr0eOBWA/YruN9r++kYPOnDy0eMaOVGLO7` +
			`SQwJ/6QHvf73yksJTncz`,
		"1d:cf:ab:66:8a:f6:77:fb:4c:b2:59:6f:12:cf:cb:2f",
	}

	ValidKeyFour = SSHKey{
		`ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCSEDMH5RyjGtEMIqM2RiPYYQgUK` +
			`9wdHCo1/AXkuQ7m1iVjHhACp8Oawf2Grn7hO4e0JUn5FaEZOnDj/9HB2VPw` +
			`EDGBwSN1caVC3yrTVkqQcsxBY9nTV+spQQMsePOdUZALcoEilvAcLRETbyn` +
			`rybaS2bfzpqbA9MEEaKQKLKGdgqiMdNXAj5I/ik/BPp0ziOMlMl1A1zilnS` +
			`UXubs1U49WWV0A70vAASvZVTXr3zrPAmstH+9Ik6FdpeE99um08FXxKYWqZ` +
			`6rZF1M6L1/SqC7ediYdVgRCoti85kKhi7fZBzwrGcCnxer+D0GFz++KDSNS` +
			`iAnVZxyXhmBrwnR6Q/v7`,
		"37:99:ab:96:c4:e8:f8:0b:0d:04:3e:1e:ee:66:e8:9e",
	}

	ValidKeyMulti = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDW+8zWO6qqXrHlcMK7obliuYp7D` +
		`vZBsK6rHlnbeV5Hh38Qn0GUX4Ahm6XeQ/NSx53wqkBQDGOJFY3s4w1a/hbd` +
		`PyLM2/yFXCYsj5FRf01JmUjAzWhuJMH9ViqzD//l4v8cR/pHC2B8PD6abKd` +
		`mIH+yLI9Cl3C4ICMKteG54egsUyboBOVKCDIKmWRLAak6sE5DPpqKF53NvD` +
		`cuDufWtaCfVAOrq6NW8wSQ7PAvfDh8gsG5uvZjY3gcWl9yI3EJVGFHcdxcv` +
		`4LtQI8mKdeg3JoufnEmeBJTZMoo83Gru5Z7tjv8J4JTUeQpd9uCCED1JAMe` +
		`cJSKgQ2gZMTbTshobpHr` + "\n" +
		`ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDSgfrzyGpE5eLiXusvLcxEmoE6e` +
		`SMUDvTW1dd2BZgfvUVwq+toQdZ6C0C1JmbC3X563n8fmKVUAQGo5JavzABG` +
		`Kpy90L3cwoGCFtb+A28YsT+bfuP+LdnCbFXm9c3DPJQx6Dch8prnDtzRjRV` +
		`CorbPvm35NY73liUXVF6g58Owlx5rWtb8OnoTh5KQps9JTSfyNckdV9bFxP` +
		`7bZvMyRYW5X33KaA+CQGpTNAKDHruSuKdAdaS6rBIZRvzzzSCF28BWwFL7Z` +
		`ghQo0ADlUMnqIeQ58nwRImZHpmvadsZi47aMKFeykk4JQUQlwjbM0xGi0uj` +
		`+hlaqGYbNo0Evcjn23cj`

	PartValidKeyMulti = `ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDZRvG2miYVkbWOr2I+9xHWXqALb` +
		`eBcyxAlYtbjxBRwrq8oFOw9vtIIZSO0r1FM6+JHzKhLSiPCMR/PK78ZqPgZ` +
		`fia8Y7cEZKaUWLtZUAl0RF9w8EtsA/2gpuLZErjcoIx6fzfEYFCJcLgcQSc` +
		`RlKG8VZT6tWIjvoLj9ki6unkG5YGmapkT60afhf3/vd7pCJO/uyszkQ9qU8` +
		`odUDTTlwftpJtUb8xGmzpEZJTgk1lbZKlZm5pVXwjNEodH7Je88RBzR7PBB` +
		`Jct+vf8wVJ/UEFXCnamvHLanJTcJIi/I5qRlKns65Bwb8M0HszPYmvTfFRD` +
		`ZLi3sPUmw6PJCJ0SgATd` + "\n" +
		`ssh-rsa bad key`

	MultiInvalid = `ssh-rsa bad key` + "\n" +
		`ssh-rsa also bad`

	EmptyKeyMulti = ""
)
