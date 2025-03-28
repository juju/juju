(command-juju-agree)=
# `juju agree`

```
Usage: juju agree [options] <term>

Summary:
Agree to terms.

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
-c, --controller (= "")
    Controller to operate in
--yes  (= false)
    Agree to terms non interactively

Details:
Agree to the terms required by a charm.

When deploying a charm that requires agreement to terms, use 'juju agree' to
view the terms and agree to them. Then the charm may be deployed.

Once you have agreed to terms, you will not be prompted to view them again.

Examples:
    # Displays terms for somePlan revision 1 and prompts for agreement.
    juju agree somePlan/1

    # Displays the terms for revision 1 of somePlan, revision 2 of otherPlan,
    # and prompts for agreement.
    juju agree somePlan/1 otherPlan/2

    # Agrees to the terms without prompting.
    juju agree somePlan/1 otherPlan/2 --yes
```