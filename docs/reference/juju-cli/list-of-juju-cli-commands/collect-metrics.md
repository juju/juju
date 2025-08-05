> See also: [metrics](#metrics)

## Summary
Collect metrics on the given unit/application.

## Usage
```juju collect-metrics [options] [application or unit]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju collect-metrics myapp

    juju collect-metrics myapp/0


## Details

Trigger metrics collection

This command waits for the metric collection to finish before returning.
You may abort this command and it will continue to run asynchronously.
Results may be checked by 'juju show-task'.



