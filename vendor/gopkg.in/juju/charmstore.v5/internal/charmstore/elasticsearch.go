// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore // import "gopkg.in/juju/charmstore.v5/internal/charmstore"

import "encoding/json"

var (
	esIndex   = mustParseJSON(esIndexJSON)
	esMapping = mustParseJSON(esMappingJSON)
)

const esSettingsVersion = 12

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
                    "type":     "nGram",
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
                },
                "lowercase_words": {
                    "type":      "custom",
                    "tokenizer": "whitespace",
                    "filter": [
                        "lowercase"
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
        "type": "multi_field",
        "fields": {
          "User": {
	    "type": "string",
	    "index": "not_analyzed",
	    "omit_norms": true,
	    "index_options": "docs"
          },
          "tok": {
	    "type": "string",
	    "analyzer": "lowercase_words",
	    "include_in_all": false
          }
        }
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
            "analyzer": "n3_20grams",
            "search_analyzer": "lowercase_words",
            "include_in_all": false
          },
          "tok": {
            "type": "string",
            "analyzer": "simple",
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
	    "type": "multi_field",
	    "fields": {
	      "Categories": {
		"type": "string",
		"index": "not_analyzed",
		"omit_norms": true,
		"index_options": "docs"
	      },
	      "tok": {
		"type": "string",
		"analyzer": "lowercase_words",
		"include_in_all": false
	      }
	    }
          },
          "Tags": {
	    "type": "multi_field",
	    "fields": {
	      "Tags": {
		"type": "string",
		"index": "not_analyzed",
		"omit_norms": true,
		"index_options": "docs"
	      },
	      "tok": {
		"type": "string",
		"analyzer": "lowercase_words",
		"include_in_all": false
	      }
	    }
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
	    "type": "multi_field",
	    "fields": {
	      "Tags": {
		"type": "string",
		"index": "not_analyzed",
		"omit_norms": true,
		"index_options": "docs"
	      },
	      "tok": {
		"type": "string",
		"analyzer": "lowercase_words",
		"include_in_all": false
	      }
	    }
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
