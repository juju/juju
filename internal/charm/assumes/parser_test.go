// Copyright 2011-2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package assumes

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/semversion"
)

type ParserSuite struct{}

var _ = tc.Suite(&ParserSuite{})

func (s *ParserSuite) TestNestedExpressionUnmarshalingFromYAML(c *tc.C) {
	payload := `
assumes:
  - chips
  - any-of:
    - guacamole
    - salsa
    - any-of:
      - good-weather
      - great-music
  - all-of:
    - table
    - lazy-suzan
`[1:]

	dst := struct {
		Assumes *ExpressionTree `yaml:"assumes,omitempty"`
	}{}
	err := yaml.NewDecoder(strings.NewReader(payload)).Decode(&dst)
	c.Assert(err, jc.ErrorIsNil)

	exp := CompositeExpression{
		ExprType: AllOfExpression,
		SubExpressions: []Expression{
			FeatureExpression{Name: "chips"},
			CompositeExpression{
				ExprType: AnyOfExpression,
				SubExpressions: []Expression{
					FeatureExpression{Name: "guacamole"},
					FeatureExpression{Name: "salsa"},
					CompositeExpression{
						ExprType: AnyOfExpression,
						SubExpressions: []Expression{
							FeatureExpression{Name: "good-weather"},
							FeatureExpression{Name: "great-music"},
						},
					},
				},
			},
			CompositeExpression{
				ExprType: AllOfExpression,
				SubExpressions: []Expression{
					FeatureExpression{Name: "table"},
					FeatureExpression{Name: "lazy-suzan"},
				},
			},
		},
	}

	c.Assert(dst.Assumes.Expression, tc.DeepEquals, exp)
}

func (s *ParserSuite) TestNestedExpressionUnmarshalingFromJSON(c *tc.C) {
	payload := `
{
  "assumes": [
    "chips",
    {
      "any-of": [
        "guacamole",
        "salsa",
        {
          "any-of": [
            "good-weather",
            "great-music"
          ]
        }
      ]
    },
    {
      "all-of": [
        "table",
        "lazy-suzan"
      ]
    }
  ]
}
`[1:]

	dst := struct {
		Assumes *ExpressionTree `json:"assumes,omitempty"`
	}{}
	err := json.NewDecoder(strings.NewReader(payload)).Decode(&dst)
	c.Assert(err, jc.ErrorIsNil)

	exp := CompositeExpression{
		ExprType: AllOfExpression,
		SubExpressions: []Expression{
			FeatureExpression{Name: "chips"},
			CompositeExpression{
				ExprType: AnyOfExpression,
				SubExpressions: []Expression{
					FeatureExpression{Name: "guacamole"},
					FeatureExpression{Name: "salsa"},
					CompositeExpression{
						ExprType: AnyOfExpression,
						SubExpressions: []Expression{
							FeatureExpression{Name: "good-weather"},
							FeatureExpression{Name: "great-music"},
						},
					},
				},
			},
			CompositeExpression{
				ExprType: AllOfExpression,
				SubExpressions: []Expression{
					FeatureExpression{Name: "table"},
					FeatureExpression{Name: "lazy-suzan"},
				},
			},
		},
	}

	c.Assert(dst.Assumes.Expression, tc.DeepEquals, exp)
}

func (s *ParserSuite) TestVersionlessFeatureExprUnmarshalingFromYAML(c *tc.C) {
	payload := `
assumes:
  - chips
`[1:]

	dst := struct {
		Assumes *ExpressionTree `yaml:"assumes,omitempty"`
	}{}
	err := yaml.NewDecoder(strings.NewReader(payload)).Decode(&dst)
	c.Assert(err, jc.ErrorIsNil)

	exp := CompositeExpression{
		ExprType: AllOfExpression,
		SubExpressions: []Expression{
			FeatureExpression{Name: "chips"},
		},
	}

	c.Assert(dst.Assumes.Expression, tc.DeepEquals, exp)
}

func (s *ParserSuite) TestVersionedFeatureExprUnmarshaling(c *tc.C) {
	payload := `
assumes: # test various combinations of whitespace and version formats
  - chips >=              2000.1.2
  - chips<2042.3.4
  - k8s-api >= 1.8
  - k8s-api < 42
`[1:]

	dst := struct {
		Assumes *ExpressionTree `yaml:"assumes,omitempty"`
	}{}
	err := yaml.NewDecoder(strings.NewReader(payload)).Decode(&dst)
	c.Assert(err, jc.ErrorIsNil)

	exp := CompositeExpression{
		ExprType: AllOfExpression,
		SubExpressions: []Expression{
			FeatureExpression{
				Name:       "chips",
				Constraint: VersionGTE,
				Version: &semversion.Number{
					Major: 2000,
					Minor: 1,
					Patch: 2,
				},
				rawVersion: "2000.1.2",
			},
			FeatureExpression{
				Name:       "chips",
				Constraint: VersionLT,
				Version: &semversion.Number{
					Major: 2042,
					Minor: 3,
					Patch: 4,
				},
				rawVersion: "2042.3.4",
			},
			FeatureExpression{
				Name:       "k8s-api",
				Constraint: VersionGTE,
				Version: &semversion.Number{
					Major: 1,
					Minor: 8,
				},
				rawVersion: "1.8",
			},
			FeatureExpression{
				Name:       "k8s-api",
				Constraint: VersionLT,
				Version: &semversion.Number{
					Major: 42,
				},
				rawVersion: "42",
			},
		},
	}

	c.Assert(dst.Assumes.Expression, tc.DeepEquals, exp)
}

func (s *ParserSuite) TestMalformedCompositeExpression(c *tc.C) {
	payload := `
assumes:
  - root:
    - access
`[1:]

	dst := struct {
		Assumes *ExpressionTree `yaml:"assumes,omitempty"`
	}{}
	err := yaml.NewDecoder(strings.NewReader(payload)).Decode(&dst)
	c.Assert(err, tc.ErrorMatches, `.*expected an "any-of" or "all-of" block.*`)
}

func (s *ParserSuite) TestFeatureExprParser(c *tc.C) {
	specs := []struct {
		descr  string
		in     string
		expErr string
	}{
		{
			descr: "feature without version",
			in:    "k8s",
		},
		{
			descr: "feature with GTE version constraint",
			in:    "juju >= 1.2.3",
		},
		{
			descr: "feature with LT version constraint",
			in:    "juju < 1.2.3",
		},
		{
			descr:  "feature with incorrect prefix",
			in:     "0day",
			expErr: ".*malformed.*",
		},
		{
			descr:  "feature with incorrect prefix (II)",
			in:     "-day",
			expErr: ".*malformed.*",
		},
		{
			descr:  "feature with incorrect suffix",
			in:     "a-day-",
			expErr: ".*malformed.*",
		},
		{
			descr:  "feature with bogus version constraint",
			in:     "popcorn = 1.0.0",
			expErr: ".*malformed.*",
		},
		{
			descr: "feature with only major version component",
			in:    "popcorn >= 1",
		},
		{
			descr: "feature with only major/minor version component",
			in:    "popcorn >= 1.2",
		},
	}

	for specIdx, spec := range specs {
		c.Logf("%d. %s", specIdx, spec.descr)
		_, err := parseFeatureExpr(spec.in)
		if spec.expErr == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, tc.ErrorMatches, spec.expErr)
		}
	}
}

func (s *ParserSuite) TestMarshalToYAML(c *tc.C) {
	payload := `
assumes:
- chips
- any-of:
  - guacamole
  - salsa
  - any-of:
    - good-weather
    - great-music
- all-of:
  - table
  - lazy-suzan
`[1:]

	dst := struct {
		Assumes *ExpressionTree `yaml:"assumes,omitempty"`
	}{}
	err := yaml.NewDecoder(strings.NewReader(payload)).Decode(&dst)
	c.Assert(err, jc.ErrorIsNil)

	var buf bytes.Buffer
	err = yaml.NewEncoder(&buf).Encode(dst)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(buf.String(), tc.Equals, payload, tc.Commentf("serialized assumes block not matching original input"))
}

func (s *ParserSuite) TestMarshalToJSON(c *tc.C) {
	payload := `
{
  "assumes": [
    "chips",
    {
      "any-of": [
        "guacamole",
        "salsa",
        {
          "any-of": [
            "good-weather",
            "great-music"
          ]
        }
      ]
    },
    {
      "all-of": [
        "table",
        "lazy-suzan"
      ]
    }
  ]
}
`[1:]

	dst := struct {
		Assumes *ExpressionTree `json:"assumes,omitempty"`
	}{}
	err := json.NewDecoder(strings.NewReader(payload)).Decode(&dst)
	c.Assert(err, jc.ErrorIsNil)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	err = enc.Encode(dst)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(buf.String(), tc.Equals, payload, tc.Commentf("serialized assumes block not matching original input"))
}
