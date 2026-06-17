## Summary
Generate the documentation for all commands

## Usage
```juju documentation [options] --out <target-folder> --no-index --split --url <base-url> --discourse-ids <filepath>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--discourse-ids` |  | File containing a mapping of commands and their discourse ids |
| `--no-index` | false | Do not generate the commands index |
| `--out` |  | Documentation output folder if not set the result is displayed using the standard output |
| `--split` | false | Generate a separate Markdown file for each command |
| `--url` |  | Documentation host URL |

## Examples

    juju documentation
    juju documentation --split 
    juju documentation --split --no-index --out /tmp/docs

To render markdown documentation using a list of existing
commands, you can use a file with the following syntax

    command1: id1
    command2: id2
    commandN: idN

For example:

    add-cloud: 1183
    add-secret: 1284
    remove-cloud: 4344

Then, the urls will be populated using the ids indicated
in the file above.

    juju documentation --split --no-index --out /tmp/docs --discourse-ids /tmp/docs/myids


## Details

This command generates a markdown formatted document with all the commands, their descriptions, arguments, and examples.



