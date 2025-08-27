(command-juju-list-actions)=
# `juju list-actions`
> See also: [run](#run), [show-action](#show-action)

**Aliases:** list-actions

## Summary
List actions defined for an application.

## Usage
```juju actions [options] <application>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | default | Specify output format (default&#x7c;json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |
| `--schema` | false | Display the full action schema |

## Examples

    juju actions postgresql
    juju actions postgresql --format yaml
    juju actions postgresql --schema


## Details

List the actions available to run on the target application, with a short
description.