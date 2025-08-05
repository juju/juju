## Summary
Print the Juju Dashboard URL, or open the Juju Dashboard in the default browser.

## Usage
```juju dashboard [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--browser` | false | Open the web browser, instead of just printing the Juju Dashboard URL |
| `--hide-credential` | false | Do not show admin credential to use for logging into the Juju Dashboard |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--port` | 31666 | Local port used to serve the dashboard |

## Examples

Print the Juju Dashboard URL and show admin credential to use to log into it:

	juju dashboard

Print the Juju Dashboard URL only:

	juju dashboard --hide-credential

Open the Juju Dashboard in the default browser and show admin credential to use to log into it:

	juju dashboard --browser

Open the Juju Dashboard in the default browser without printing the login credential:

	juju dashboard --hide-credential --browser

An error is returned if the Juju Dashboard is not running.


## Details




