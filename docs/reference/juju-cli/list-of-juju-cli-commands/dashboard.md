(command-juju-dashboard)=
# `juju dashboard`

```
Usage: juju dashboard [options]

Summary:
Print the Juju Dashboard URL, or open the Juju Dashboard in the default browser.

Global Options:
--debug  (= false)
    equivalent to --show-log --logging-config=<root>=DEBUG
-h, --help  (= false)
    Show help on a command or other topic.
--logging-config (= "")
    specify log levels for modules
--quiet  (= false)
    show no informational output
--show-log  (= false)
    if set, write the log file to stderr
--verbose  (= false)
    show more verbose output

Command Options:
-B, --no-browser-login  (= false)
    Do not use web browser for authentication
--browser  (= false)
    Open the web browser, instead of just printing the Juju Dashboard URL
--hide-credential  (= false)
    Do not show admin credential to use for logging into the Juju Dashboard
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>

Details:
Print the Juju Dashboard URL and show admin credential to use to log into it:

	juju dashboard

Print the Juju Dashboard URL only:

	juju dashboard --hide-credential

Open the Juju Dashboard in the default browser and show admin credential to use to log into it:

	juju dashboard --browser

Open the Juju Dashboard in the default browser without printing the login credential:

	juju dashboard --hide-credential --browser

An error is returned if the Juju Dashboard is not available in the controller.

Aliases: gui
```