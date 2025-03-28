<!--
The PR title should match: <type>(optional <scope>): <description>.

Please also ensure all commits in this PR comply with our conventional commits specification:
https://github.com/juju/juju/blob/main/docs/contributor/reference/conventional-commits.md
-->

<!-- Why this change is needed and what it does. -->

## Checklist

<!-- If an item is not applicable, use `~strikethrough~`. -->

- [ ] Code style: imports ordered, good names, simple structure, etc
- [ ] Comments saying why design decisions were made
- [ ] Go unit tests, with comments saying what you're testing
- [ ] [Integration tests](https://github.com/juju/juju/tree/main/tests), with comments saying what you're testing
- [ ] [doc.go](https://discourse.charmhub.io/t/readme-in-packages/451) added or updated in changed packages

## QA steps

<!--

Describe steps to verify that the change works.

If you're changing any of the facades, you need to ensure that you've tested
a model migration from 3.6 to 4.0 and from 4.0 to 4.0.

The following steps are a good starting point:

 1. Bootstrap a 3.6 controller and deploy a charm.

```sh
$ juju bootstrap lxd src36
$ juju add-model moveme1
$ juju deploy juju-qa-test
```

 2. Bootstrap a 4.0 controller with the changes and migrate the model.

```sh
$ juju bootstrap lxd dst40
$ juju migrate src36:moveme1 dst40
$ juju add-unit juju-qa-test
```

 3. Verify no errors exist in the model logs for the agents. If there are
    errors, this is a bug and should be fixed before merging. The fix can
    either be applied to the 4.0 branch (preferable) or the 3.6 branch, though
    that needs to be discussed with the team.

```sh
$ juju debug-log -m dst40:controller
$ juju debug-log -m dst40:moveme1
```

    4. We also need to test a model migration from 4.0 to 4.0.

```sh
$ juju bootstrap lxd src40
$ juju add-model moveme2
$ juju deploy juju-qa-test
```

```sh
$ juju migrate src40:moveme2 dst40
$ juju add-unit juju-qa-test
```

    5. Verify that there are no errors in the controller or model logs.

```sh
$ juju debug-log -m dst40:controller
$ juju debug-log -m dst40:moveme2
```

-->

## Documentation changes

<!-- How it affects user workflow (CLI or API). -->

## Links

<!-- Link to all relevant specification, documentation, bug, issue or JIRA card. -->

<!-- Replace #19267 with an issue reference or link, otherwise remove this line. -->
**Issue:** Fixes #19267.

<!-- Add a Jira card reference or link, otherwise remove this line. -->
**Jira card:** JUJU-
