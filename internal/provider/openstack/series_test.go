// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/go-goose/goose/v5/identity"
	"github.com/go-goose/goose/v5/nova"
	"github.com/go-goose/goose/v5/swift"
	"github.com/juju/errors"
	"github.com/juju/tc"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	envstorage "github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
)

// MetadataStorage returns a Storage instance which is used to store simplestreams metadata for tests.
func MetadataStorage(e environs.Environ) envstorage.Storage {
	env := e.(*Environ)
	ecfg := env.ecfg()
	container := "juju-dist-test"

	factory := NewClientFactory(env.cloud(), ecfg)
	newClient, err := factory.getClientState()
	if err != nil {
		panic(fmt.Errorf("cannot create %s container: %v", container, err))
	}

	metadataStorage := &openstackstorage{
		containerName: container,
		swift:         swift.New(newClient),
	}

	// Ensure the container exists.
	err = metadataStorage.makeContainer(container, swift.PublicRead)
	if err != nil {
		panic(fmt.Errorf("cannot create %s container: %v", container, err))
	}
	return metadataStorage
}

func InstanceAddress(c *tc.C, publicIP string, addresses map[string][]nova.IPAddress) string {
	addr, _ := convertNovaAddresses(c.Context(), publicIP, addresses).OneMatchingScope(network.ScopeMatchPublic)
	return addr.Value
}

// Include images for arches currently supported.  i386 is no longer
// supported, so it can be excluded.
// TODO (stickupkid): Refactor this to actually build this for the given LTS.
var indexData = `
{
  "index": {
  "com.ubuntu.cloud:released:openstack": {
    "updated": "Wed, 01 May 2013 13:31:26 +0000",
    "clouds": [
  {
    "region": "{{.Region}}",
    "endpoint": "{{.URL}}"
  }
    ],
    "cloudname": "test",
    "datatype": "image-ids",
    "format": "products:1.0",
    "products": [
      "com.ubuntu.cloud:server:24.04:s390x",
      "com.ubuntu.cloud:server:24.04:amd64",
      "com.ubuntu.cloud:server:24.04:arm64",
      "com.ubuntu.cloud:server:24.04:ppc64el",
      "com.ubuntu.cloud:server:22.04:s390x",
      "com.ubuntu.cloud:server:22.04:amd64",
      "com.ubuntu.cloud:server:22.04:arm64",
      "com.ubuntu.cloud:server:22.04:ppc64el",
      "com.ubuntu.cloud:server:20.04:s390x",
      "com.ubuntu.cloud:server:20.04:amd64",
      "com.ubuntu.cloud:server:20.04:arm64",
      "com.ubuntu.cloud:server:20.04:ppc64el",
      "com.ubuntu.cloud:server:18.04:s390x",
      "com.ubuntu.cloud:server:18.04:amd64",
      "com.ubuntu.cloud:server:18.04:arm64",
      "com.ubuntu.cloud:server:18.04:ppc64el",
      "com.ubuntu.cloud:server:16.04:s390x",
      "com.ubuntu.cloud:server:16.04:amd64",
      "com.ubuntu.cloud:server:16.04:arm64",
      "com.ubuntu.cloud:server:16.04:ppc64el",
      "com.ubuntu.cloud:server:14.04:s390x",
      "com.ubuntu.cloud:server:14.04:amd64",
      "com.ubuntu.cloud:server:14.04:arm64",
      "com.ubuntu.cloud:server:14.04:ppc64el",
      "com.ubuntu.cloud:server:12.10:amd64",
      "com.ubuntu.cloud:server:13.04:amd64"
    ],
    "path": "image-metadata/products.json"
  }
  },
  "updated": "Wed, 01 May 2013 13:31:26 +0000",
  "format": "index:1.0"
}
`

var imagesData = `
{
  "content_id": "com.ubuntu.cloud:released:openstack",
  "products": {
    "com.ubuntu.cloud:server:24.04:amd64": {
      "release": "noble",
      "version": "24.04",
      "arch": "amd64",
      "versions": {
        "20121218": {
          "items": {
            "inst1": {
              "region": "some-region",
              "id": "1"
            },
            "inst2": {
              "region": "another-region",
              "id": "2"
            }
          },
          "pubname": "ubuntu-noble-24.04-amd64-server-20121218",
          "label": "release"
        },
        "20121111": {
          "items": {
            "inst3": {
              "region": "some-region",
              "id": "3"
            }
          },
          "pubname": "ubuntu-noble-24.04-amd64-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:24.04:arm64": {
      "release": "noble",
      "version": "24.04",
      "arch": "arm64",
      "versions": {
        "20121111": {
          "items": {
            "inst1604arm64": {
              "region": "some-region",
              "id": "id-1604arm64"
            }
          },
          "pubname": "ubuntu-noble-24.04-arm64-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:24.04:ppc64el": {
      "release": "noblr",
      "version": "24.04",
      "arch": "ppc64el",
      "versions": {
        "20121111": {
          "items": {
            "inst1604ppc64el": {
              "region": "some-region",
              "id": "id-1604ppc64el"
            }
          },
          "pubname": "ubuntu-noble-24.04-ppc64el-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:24.04:s390x": {
      "release": "noble",
      "version": "24.04",
      "arch": "s390x",
      "versions": {
        "20121111": {
          "items": {
            "inst1604s390x": {
              "region": "some-region",
              "id": "id-1604s390x"
            }
          },
          "pubname": "ubuntu-noble-24.04-s390x-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:22.04:amd64": {
      "release": "jammy",
      "version": "22.04",
      "arch": "amd64",
      "versions": {
        "20121218": {
          "items": {
            "inst1": {
              "region": "some-region",
              "id": "1"
            },
            "inst2": {
              "region": "another-region",
              "id": "2"
            }
          },
          "pubname": "ubuntu-jammy-22.04-amd64-server-20121218",
          "label": "release"
        },
        "20121111": {
          "items": {
            "inst3": {
              "region": "some-region",
              "id": "3"
            }
          },
          "pubname": "ubuntu-jammy-22.04-amd64-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:22.04:arm64": {
      "release": "jammy",
      "version": "22.04",
      "arch": "arm64",
      "versions": {
        "20121111": {
          "items": {
            "inst1604arm64": {
              "region": "some-region",
              "id": "id-1604arm64"
            }
          },
          "pubname": "ubuntu-jammy-22.04-arm64-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:22.04:ppc64el": {
      "release": "jammy",
      "version": "22.04",
      "arch": "ppc64el",
      "versions": {
        "20121111": {
          "items": {
            "inst1604ppc64el": {
              "region": "some-region",
              "id": "id-1604ppc64el"
            }
          },
          "pubname": "ubuntu-jammy-22.04-ppc64el-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:22.04:s390x": {
      "release": "jammy",
      "version": "22.04",
      "arch": "s390x",
      "versions": {
        "20121111": {
          "items": {
            "inst1604s390x": {
              "region": "some-region",
              "id": "id-1604s390x"
            }
          },
          "pubname": "ubuntu-jammy-22.04-s390x-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:20.04:amd64": {
      "release": "focal",
      "version": "20.04",
      "arch": "amd64",
      "versions": {
        "20121218": {
          "items": {
            "inst1": {
              "region": "some-region",
              "id": "1"
            },
            "inst2": {
              "region": "another-region",
              "id": "2"
            }
          },
          "pubname": "ubuntu-focal-20.04-amd64-server-20121218",
          "label": "release"
        },
        "20121111": {
          "items": {
            "inst3": {
              "region": "some-region",
              "id": "3"
            }
          },
          "pubname": "ubuntu-focal-20.04-amd64-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:20.04:arm64": {
      "release": "focal",
      "version": "20.04",
      "arch": "arm64",
      "versions": {
        "20121111": {
          "items": {
            "inst1604arm64": {
              "region": "some-region",
              "id": "id-1604arm64"
            }
          },
          "pubname": "ubuntu-focal-20.04-arm64-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:20.04:ppc64el": {
      "release": "focal",
      "version": "20.04",
      "arch": "ppc64el",
      "versions": {
        "20121111": {
          "items": {
            "inst1604ppc64el": {
              "region": "some-region",
              "id": "id-1604ppc64el"
            }
          },
          "pubname": "ubuntu-focal-20.04-ppc64el-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:20.04:s390x": {
      "release": "focal",
      "version": "20.04",
      "arch": "s390x",
      "versions": {
        "20121111": {
          "items": {
            "inst1604s390x": {
              "region": "some-region",
              "id": "id-1604s390x"
            }
          },
          "pubname": "ubuntu-focal-20.04-s390x-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:18.04:amd64": {
      "release": "bionic",
      "version": "18.04",
      "arch": "amd64",
      "versions": {
        "20121218": {
          "items": {
            "inst1": {
              "region": "some-region",
              "id": "1"
            },
            "inst2": {
              "region": "another-region",
              "id": "2"
            }
          },
          "pubname": "ubuntu-bionic-18.04-amd64-server-20121218",
          "label": "release"
        },
        "20121111": {
          "items": {
            "inst3": {
              "region": "some-region",
              "id": "3"
            }
          },
          "pubname": "ubuntu-bionic-18.04-amd64-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:18.04:arm64": {
      "release": "bionic",
      "version": "18.04",
      "arch": "arm64",
      "versions": {
        "20121111": {
          "items": {
            "inst1604arm64": {
              "region": "some-region",
              "id": "id-1604arm64"
            }
          },
          "pubname": "ubuntu-bionic-18.04-arm64-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:18.04:ppc64el": {
      "release": "bionic",
      "version": "18.04",
      "arch": "ppc64el",
      "versions": {
        "20121111": {
          "items": {
            "inst1604ppc64el": {
              "region": "some-region",
              "id": "id-1604ppc64el"
            }
          },
          "pubname": "ubuntu-bionic-18.04-ppc64el-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:18.04:s390x": {
      "release": "bionic",
      "version": "18.04",
      "arch": "s390x",
      "versions": {
        "20121111": {
          "items": {
            "inst1604s390x": {
              "region": "some-region",
              "id": "id-1604s390x"
            }
          },
          "pubname": "ubuntu-bionic-18.04-s390x-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:16.04:amd64": {
      "release": "trusty",
      "version": "16.04",
      "arch": "amd64",
      "versions": {
        "20121218": {
          "items": {
            "inst1": {
              "region": "some-region",
              "id": "1"
            },
            "inst2": {
              "region": "another-region",
              "id": "2"
            }
          },
          "pubname": "ubuntu-trusty-16.04-amd64-server-20121218",
          "label": "release"
        },
        "20121111": {
          "items": {
            "inst3": {
              "region": "some-region",
              "id": "3"
            }
          },
          "pubname": "ubuntu-trusty-16.04-amd64-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:16.04:arm64": {
      "release": "xenial",
      "version": "16.04",
      "arch": "arm64",
      "versions": {
        "20121111": {
          "items": {
            "inst1604arm64": {
              "region": "some-region",
              "id": "id-1604arm64"
            }
          },
          "pubname": "ubuntu-xenial-16.04-arm64-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:16.04:ppc64el": {
      "release": "xenial",
      "version": "16.04",
      "arch": "ppc64el",
      "versions": {
        "20121111": {
          "items": {
            "inst1604ppc64el": {
              "region": "some-region",
              "id": "id-1604ppc64el"
            }
          },
          "pubname": "ubuntu-xenial-16.04-ppc64el-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:14.04:amd64": {
      "release": "trusty",
      "version": "14.04",
      "arch": "amd64",
      "versions": {
        "20121218": {
          "items": {
            "inst1": {
              "region": "some-region",
              "id": "1"
            },
            "inst2": {
              "region": "another-region",
              "id": "2"
            }
          },
          "pubname": "ubuntu-trusty-14.04-amd64-server-20121218",
          "label": "release"
        },
        "20121111": {
          "items": {
            "inst3": {
              "region": "some-region",
              "id": "3"
            }
          },
          "pubname": "ubuntu-trusty-14.04-amd64-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:14.04:arm64": {
      "release": "trusty",
      "version": "14.04",
      "arch": "arm64",
      "versions": {
        "20121111": {
          "items": {
            "inst33": {
              "region": "some-region",
              "id": "33"
            }
          },
          "pubname": "ubuntu-trusty-14.04-arm64-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:14.04:ppc64el": {
      "release": "trusty",
      "version": "14.04",
      "arch": "ppc64el",
      "versions": {
        "20121111": {
          "items": {
            "inst33": {
              "region": "some-region",
              "id": "33"
            }
          },
          "pubname": "ubuntu-trusty-14.04-ppc64el-server-20121111",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:12.10:amd64": {
      "release": "quantal",
      "version": "12.10",
      "arch": "amd64",
      "versions": {
        "20121218": {
          "items": {
            "inst3": {
              "region": "region-1",
              "id": "id-1"
            },
            "inst4": {
              "region": "region-2",
              "id": "id-2"
            }
          },
          "pubname": "ubuntu-quantal-12.14-amd64-server-20121218",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:13.04:amd64": {
      "release": "raring",
      "version": "13.04",
      "arch": "amd64",
      "versions": {
        "20121218": {
          "items": {
            "inst5": {
              "region": "some-region",
              "id": "id-y"
            },
            "inst6": {
              "region": "another-region",
              "id": "id-z"
            }
          },
          "pubname": "ubuntu-raring-13.04-amd64-server-20121218",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:14.04:s390x": {
      "release": "trusty",
      "version": "14.04",
      "arch": "s390x",
      "versions": {
        "20121218": {
          "items": {
            "inst5": {
              "region": "some-region",
              "id": "id-y"
            },
            "inst6": {
              "region": "another-region",
              "id": "id-z"
            }
          },
          "pubname": "ubuntu-trusty-14.04-s390x-server-20121218",
          "label": "release"
        }
      }
    },
    "com.ubuntu.cloud:server:16.04:s390x": {
      "release": "xenial",
      "version": "16.04",
      "arch": "s390x",
      "versions": {
        "20121218": {
          "items": {
            "inst5": {
              "region": "some-region",
              "id": "id-y"
            },
            "inst6": {
              "region": "another-region",
              "id": "id-z"
            }
          },
          "pubname": "ubuntu-xenial-16.04-s390x-server-20121218",
          "label": "release"
        }
      }
    }
  },
  "format": "products:1.0"
}
`

const productMetadatafile = "image-metadata/products.json"

func UseTestImageData(stor envstorage.Storage, cred *identity.Credentials) {
	// Put some image metadata files into the public storage.
	t := template.Must(template.New("").Parse(indexData))
	var metadata bytes.Buffer
	if err := t.Execute(&metadata, cred); err != nil {
		panic(fmt.Errorf("cannot generate index metdata: %v", err))
	}
	data := metadata.Bytes()
	stor.Put(simplestreams.UnsignedIndex("v1", 1), bytes.NewReader(data), int64(len(data)))
	stor.Put(
		productMetadatafile, strings.NewReader(imagesData), int64(len(imagesData)))

	envtesting.SignTestTools(stor)
}

func RemoveTestImageData(stor envstorage.Storage) {
	stor.RemoveAll()
}

// DiscardSecurityGroup cleans up a security group, it is not an error to
// delete something that doesn't exist.
func DiscardSecurityGroup(e environs.Environ, name string) error {
	env := e.(*Environ)
	neutronClient := env.neutron()
	groups, err := neutronClient.SecurityGroupByNameV2(name)
	if err != nil || len(groups) == 0 {
		if errors.Is(err, errors.NotFound) {
			// Group already deleted, done
			return nil
		}
	}
	for _, group := range groups {
		err = neutronClient.DeleteSecurityGroupV2(group.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

func FindInstanceSpec(
	e environs.Environ,
	base corebase.Base, arch, cons string,
	imageMetadata []*imagemetadata.ImageMetadata,
) (spec *instances.InstanceSpec, err error) {
	env := e.(*Environ)
	return findInstanceSpec(env, instances.InstanceConstraint{
		Base:        base,
		Arch:        arch,
		Region:      env.cloud().Region,
		Constraints: constraints.MustParse(cons),
	}, imageMetadata)
}
