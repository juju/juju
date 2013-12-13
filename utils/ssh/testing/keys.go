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
)
