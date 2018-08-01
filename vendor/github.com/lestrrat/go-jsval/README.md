# go-jsval

Validator toolset, with code generation from JSON Schema

[![Build Status](https://travis-ci.org/lestrrat/go-jsval.svg?branch=master)](https://travis-ci.org/lestrrat/go-jsval)

[![GoDoc](https://godoc.org/github.com/lestrrat/go-jsval?status.svg)](https://godoc.org/github.com/lestrrat/go-jsval)


# Description

The `go-jsval` package is a data validation toolset, with
a tool to generate validators in Go from JSON schemas.

# Synopsis

Read a schema file and create a validator using `jsval` command:

```shell
jsval -s /path/to/schema.json -o validator.go
jsval -s /path/to/hyperschema.json -o validator.go -p "/links/0/schema" -p "/links/1/schema" -p "/links/2/schema"
```

Read a schema file and create a validator programatically:

```go
package jsval_test

import (
  "log"

  "github.com/lestrrat/go-jsschema"
  "github.com/lestrrat/go-jsval/builder"
)

func ExampleBuild() {
  s, err := schema.ReadFile(`/path/to/schema.json`)
  if err != nil {
    log.Printf("failed to open schema: %s", err)
    return
  }

  b := builder.New()
  v, err := b.Build(s)
  if err != nil {
    log.Printf("failed to build validator: %s", err)
    return
  }

  var input interface{}
  if err := v.Validate(input); err != nil {
    log.Printf("validation failed: %s", err)
    return
  }
}
```

Build a validator by hand:

```go
func ExampleManual() {
  v := jsval.Object().
    AddProp(`zip`, jsval.String().RegexpString(`^\d{5}$`)).
    AddProp(`address`, jsval.String()).
    AddProp(`name`, jsval.String()).
    AddProp(`phone_number`, jsval.String().RegexpString(`^[\d-]+$`)).
    Required(`zip`, `address`, `name`)

  var input interface{}
  if err := v.Validate(input); err != nil {
    log.Printf("validation failed: %s", err)
    return
  }
}
```

# Install

```
go get -u github.com/lestrrat/go-jsval
```

If you want to install the `jsval` tool, do

```
go get -u github.com/lestrrat/go-jsval/cmd/jsval
```

# Features

## Can generate validators from JSON Schema definition

The following command creates a file named `jsval.go` 
which contains various variables containing `*jsval.JSVal`
structures so you can include them in your code:

```
jsval -s schema.json -o jsval.go
```

See the file `generated_validator_test.go` for a sample
generated from JSON Schema schema.

If your document isn't a real JSON schema but contains one
or more JSON schema (like JSON Hyper Schema) somewhere inside
the document, you can use the `-p` argument to access a
specific portion of a JSON document:

```
jsval -s hyper.json -p "/links/0" -p "/lnks/1"
```

This will generate a set of validators, with JSON references
within the file `hyper.json` properly resolved.

## Can handle JSON References in JSON Schema definitions

Note: Not very well tested. Test cases welcome

This packages tries to handle JSON References properly.
For example, in the schema below, "age" input is validated
against the `positiveInteger` schema:

```json
{
  "definitions": {
    "positiveInteger": {
      "type": "integer",
      "minimum": 0,
    }
  },
  "properties": {
    "age": { "$ref": "#/definitions/positiveInteger" }
  }
}
```

## Run a playground server

```
jsval server -listen :8080
```

You can specify a JSON schema, and see what kind of validator gets generated.

# Tricks

## Specifying structs with values that may or may not be initialized

With maps, it's easy to check if a property exists. But if you are validating a struct,
however, all of the fields exist all the time, and you basically cannot detect if you
have a missing field to apply defaults, etc.

For such cases you should use the `Maybe` interface provided in this package:

```go
type Foo struct {
  Name MaybeString `json:"name"`
}
```

This will declare the value as "optional", and the JSVal validation mechanism does
the correct thing to process this field.

# References

| Name                                                     | Notes                            |
|:--------------------------------------------------------:|:---------------------------------|
| [go-jsschema](https://github.com/lestrrat/go-jsschema)   | JSON Schema implementation       |
| [go-jshschema](https://github.com/lestrrat/go-jshschema) | JSON Hyper Schema implementation |
| [go-jsref](https://github.com/lestrrat/go-jsref)         | JSON Reference implementation    |
| [go-jspointer](https://github.com/lestrrat/go-jspointer) | JSON Pointer implementations     |

