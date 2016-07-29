// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore // import "gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"

import "encoding/json"

var (
	esIndex   = mustParseJSON(esIndexJSON)
	esMapping = mustParseJSON(esMappingJSON)
)

const esSettingsVersion = 9

func mustParseJSON(s string) interface{} {
	var j json.RawMessage
	if err := json.Unmarshal([]byte(s), &j); err != nil {
		panic(err)
	}
	return &j
}

const esIndexJSON = `
{
    "settings": {
        "number_of_shards": 1,
        "analysis": {
            "filter": {
                "n3_20grams_filter": {
                    "type":     "edgeNGram",
                    "min_gram": 3,
                    "max_gram": 20
                }
            },
            "analyzer": {
                "n3_20grams": {
                    "type":      "custom",
                    "tokenizer": "keyword",
                    "filter": [
                        "lowercase",
                        "n3_20grams_filter"
                    ]
                }
            }
        }
    }
}
`

const esMappingJSON = `
{
  "entity": {
    "dynamic": "false",
    "properties": {
      "URL": {
        "type": "string",
        "index": "not_analyzed",
        "omit_norms": true,
        "index_options": "docs"
      },
      "PromulgatedURL": {
        "type": "string",
        "index": "not_analyzed",
        "index_options": "docs"
      },
      "BaseURL": {
        "type": "string",
        "index": "not_analyzed",
        "index_options": "docs"
      },
      "User": {
        "type": "string",
        "index": "not_analyzed",
        "omit_norms": true,
        "index_options": "docs"
      },
      "Name": {
        "type": "multi_field",
        "fields": {
          "Name": {
            "type": "string",
            "index": "not_analyzed",
            "omit_norms": true,
            "index_options": "docs"
          },
          "ngrams": {
            "type": "string",
            "index_analyzer": "n3_20grams",
            "include_in_all": false
          }
        }
      },
      "Revision": {
        "type": "integer",
        "index": "not_analyzed"
      },
      "Series": {
        "type": "string",
        "index": "not_analyzed",
        "omit_norms": true,
        "index_options": "docs"
      },
      "TotalDownloads": {
        "type": "long"
      },
      "BlobHash": {
        "type": "string",
        "index": "not_analyzed",
        "omit_norms": true,
        "index_options": "docs"
      },
      "UploadTime": {
        "type": "date",
        "format": "dateOptionalTime"
      },
      "CharmMeta": {
        "dynamic": "false",
        "properties": {
          "Name": {
            "type": "string"
          },
          "Summary": {
            "type": "string"
          },
          "Description": {
            "type": "string"
          },
          "Provides": {
            "dynamic": "false",
            "properties": {
              "Name": {
                "type": "string",
                "index": "not_analyzed",
                "omit_norms": true,
                "index_options": "docs"
              },
              "Role": {
                "type": "string",
                "index": "not_analyzed",
                "omit_norms": true,
                "index_options": "docs"
              },
              "Interface": {
                "type": "string",
                "index": "not_analyzed",
                "omit_norms": true,
                "index_options": "docs"
              },
              "Scope": {
                "type": "string",
                "index": "not_analyzed",
                "omit_norms": true,
                "index_options": "docs"
              }
            }
          },
          "Requires": {
            "dynamic": "false",
            "properties": {
              "Name": {
                "type": "string",
                "index": "not_analyzed",
                "omit_norms": true,
                "index_options": "docs"
              },
              "Role": {
                "type": "string",
                "index": "not_analyzed",
                "omit_norms": true,
                "index_options": "docs"
              },
              "Interface": {
                "type": "string",
                "index": "not_analyzed",
                "omit_norms": true,
                "index_options": "docs"
              },
              "Scope": {
                "type": "string",
                "index": "not_analyzed",
                "omit_norms": true,
                "index_options": "docs"
              }
            }
          },
          "Peers": {
            "dynamic": "false",
            "properties": {
              "Name": {
                "type": "string",
                "index": "not_analyzed",
                "omit_norms": true,
                "index_options": "docs"
              },
              "Role": {
                "type": "string",
                "index": "not_analyzed",
                "omit_norms": true,
                "index_options": "docs"
              },
              "Interface": {
                "type": "string",
                "index": "not_analyzed",
                "omit_norms": true,
                "index_options": "docs"
              },
              "Scope": {
                "type": "string",
                "index": "not_analyzed",
                "omit_norms": true,
                "index_options": "docs"
              }
            }
          },
          "Categories": {
            "type": "string",
            "index": "not_analyzed",
            "omit_norms": true,
            "index_options": "docs"
          },
          "Tags": {
            "type": "string",
            "index": "not_analyzed",
            "omit_norms": true,
            "index_options": "docs"
          }
        }
      },
      "charmactions": {
        "dynamic": "false",
        "properties": {
          "description": {
            "type": "string"
          },
          "action_name": {
            "type": "string",
            "index": "not_analyzed",
            "omit_norms": true,
            "index_options": "docs"
          }
        }
      },
      "CharmProvidedInterfaces": {
        "type": "string",
        "index": "not_analyzed",
        "omit_norms": true,
        "index_options": "docs"
      },
      "CharmRequiredInterfaces": {
        "type": "string",
        "index": "not_analyzed",
        "omit_norms": true,
        "index_options": "docs"
      },
      "BundleData": {
        "type": "object",
        "dynamic": "false",
        "properties": {
          "Services": {
            "type": "object",
            "dynamic": "false",
            "properties": {
              "Charm": {
                "type": "string",
                "index": "not_analyzed",
                "omit_norms": true,
                "index_options": "docs"
              },
              "NumUnits": {
                "type": "integer",
                "index": "not_analyzed"
              }
            }
          },
          "Applications": {
            "type": "object",
            "dynamic": "false",
            "properties": {
              "Charm": {
                "type": "string",
                "index": "not_analyzed",
                "omit_norms": true,
                "index_options": "docs"
              },
              "NumUnits": {
                "type": "integer",
                "index": "not_analyzed"
              }
            }
          },
          "Series": {
            "type": "string"
          },
          "Relations": {
            "type": "string",
            "index": "not_analyzed"
          },
          "Tags": {
            "type": "string",
            "index": "not_analyzed",
            "omit_norms": true,
            "index_options": "docs"
          }
        }
      },
      "BundleReadMe": {
        "type": "string",
        "index": "not_analyzed",
        "omit_norms": true,
        "index_options": "docs"
      },
      "BundleCharms": {
        "type": "string",
        "index": "not_analyzed",
        "omit_norms": true,
        "index_options": "docs"
      },
      "BundleMachineCount": {
        "type": "integer"
      },
      "BundleUnitCount": {
        "type": "integer"
      },
      "Public": {
        "type": "boolean",
        "index": "not_analyzed",
        "omit_norms": true,
        "index_options": "docs"
      },
      "ReadACLs": {
        "type": "string",
        "index": "not_analyzed",
        "omit_norms": true,
        "index_options": "docs"
      },
      "SingleSeries": {
        "type": "boolean",
        "index": "not_analyzed",
        "omit_norms": true,
        "index_options": "docs"
      },
      "AllSeries": {
        "type": "boolean",
        "index": "not_analyzed",
        "omit_norms": true,
        "index_options": "docs"
      }
    }
  }
}
`
