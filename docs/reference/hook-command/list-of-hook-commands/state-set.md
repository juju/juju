(hook-command-state-set)=
# `state-set`
> See also: [state-delete](#state-delete), [state-get](#state-get)

## Summary
Set server-side-state values.

## Usage
``` state-set [options] key=value [key=value ...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--file` |  | file containing key-value pairs |

## Details

state-set sets the value of the server side state specified by key.

The --file option should be used when one or more key-value pairs
are too long to fit within the command length limit of the shell
or operating system. The file will contain a YAML map containing
the settings as strings.  Settings in the file will be overridden
by any duplicate key-value arguments. A value of "-" for the filename
means &lt;stdin&gt;.

The following fixed size limits apply:
- Length of stored keys cannot exceed 256 bytes.
- Length of stored values cannot exceed 65536 bytes.