(command-juju-grant-secret)=
# `juju grant-secret`
## Summary
Grant access to a secret.

## Usage
```juju grant-secret [options] <ID>|<name> <application>[,<application>...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju grant-secret my-secret ubuntu-k8s
    juju grant-secret 9m4e2mr0ui3e8a215n4g ubuntu-k8s,prometheus-k8s


## Details

Grant applications access to view the value of a specified secret.