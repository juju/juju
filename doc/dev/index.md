<!-- TODO: Assess this dead documentation (moved from discourse, maybe just remove or use it to reformat)
|| If you want to... | visit... |
|-|--|--|
|| manage charmed applications | [Juju docs](https://juju.is/docs/olm) |
|| create charmed applications | [Charm SDK docs](https://juju.is/docs/sdk) |
| :point_right: | learn how Juju works under the hood | [Juju developer docs](https://juju.is/docs/dev) |
-->

<!-- Learn how Juju works under the hood! -->

This documentation is aimed at Juju developers or Juju users who would like to see what's under the hood.
It is not intended to stand on its own but merely to supplement the [Juju documentation](https://juju.is/docs/olm) and
the [Charm SDK documentation](https://juju.is/docs/sdk. Note also that many of our Juju developer docs are still just
on [GitHub](https://github.com/juju/juju/tree/3.6/doc). <!-- TODO: This link and references may be not that useful since we are migrating this doc to Github :) -->



-----------------------------

## In this documentation

|                   |                                                                                            |
|-------------------|--------------------------------------------------------------------------------------------|
|                   | [How-to guides](how-to) </br> Step-by-step guides covering key operations and common tasks |
|                   | [Reference](reference) </br> Technical information - specifications, APIs, architecture    |

## Project and community

Juju is an open source project that warmly welcomes community projects, contributions, suggestions, fixes and
constructive feedback.

* Learn about the [Roadmap & Releases](https://discourse.charmhub.io/t/5064)
* Read our [Code of Conduct ](https://ubuntu.com/community/code-of-conduct)
* Join our [Matrix chat](https://matrix.to/#/#charmhub-jujudev:ubuntu.com)
* Join the [Discourse forum ](https://discourse.charmhub.io/t/welcome-to-the-charmed-operator-community/8) to talk
  about [Juju](https://discourse.charmhub.io/tags/c/juju/6/community-workshop), [charms](https://discourse.charmhub.io/c/charm/41), [docs](https://discourse.charmhub.io/c/doc/22),
  or [to meet the community](https://discourse.charmhub.io/tag/community-workshop)
* Report a bug on [Launchpad ](https://bugs.launchpad.net/juju) (for code)
  or [GitHub](https://github.com/juju/docs/issues) (for docs)
* Contribute to the documentation
  on [Discourse](https://discourse.charmhub.io/t/documentation-guidelines-for-contributors/1245)
* Contribute to the code on [GitHub](https://github.com/juju/juju/blob/develop/CONTRIBUTING.md)
* Visit the [Juju careers page](https://juju.is/careers)

<!-- TODO: this tab was platform specific with discourse.
## Navigation

[details=Navigation]

| Level | Path | Navlink |
|-------|----------------------------------------|---------------------------------------------------|
| 1 | | [Dev documentation](/t/6669)                      |
| 1 | how-to | [How-to guides](/t/6825)                          |
| 2 | merge-forward | [Merge forward](/t/10805)                         |
| 2 | debug-bootstrapmachine-failures | [Debug bootstrap/machine failures](/t/6835)       |
| 2 | create-a-new-mongo-db-collection | [Create a new Mongo DB collection](/t/6863)       |
| 2 | write-a-unit-test | [Write a unit test](/t/7207)                      |
| 3 | create-a-unit-test-suite | [Create a unit test suite](/t/7242)               |
| 2 | write-an-integration-test | [Write an integration test](/t/7210)              |
| 1 | reference | [Reference](/t/6824)                              |
| 2 | agent | [Agent](/t/11679)                                 |
| 2 | agent-introspection | [Agent introspection](/t/117)                     |
| 3 | agent-introspection-juju-engine-report | [juju_engine_report](/t/146)                      |
| 3 | agent-introspection-juju-goroutines | [juju_goroutines](/t/118)                         |
| 3 | agent-introspection-juju-heap-profile | [juju_heap_profile](/t/6640)                      |
| 3 | agent-introspection-juju-leases | [juju_leases](/t/5670)                            |
| 3 | agent-introspection-juju-machine-lock | [juju_machine_lock](/t/116)                       |
| 3 | agent-introspection-juju-metrics | [juju_metrics](/t/6641)                           |
| 3 | agent-introspection-juju-revoke-lease | [juju_revoke_lease](/t/5670)                      |
| 3 | agent-introspection-juju-start-unit | [juju_start_unit](/t/5667)                        |
| 3 | agent-introspection-juju-stop-unit | [juju_stop_unit](/t/5668)                         |
| 3 | agent-introspection-juju-unit-status | [juju_unit_status](/t/5666)                       |
| 2 | catacomb-package | [`catacomb`](/t/11680)                            |
| 2 | commands-available-on-a-juju-machine | [Commands available on a Juju machine](/t/2999)   |
| 2 | containeragent-binary | [`containeragent`](/t/11677)                      |
| 2 | dependency-package | [`dependency`](/t/11668)                          |
| 2 | jujud-binary | [`jujud`](/t/11674)                               |
| 2 | testing | [Testing](/t/7203)                                |
| 3 | unit-testing | [Unit testing](/t/7204)                           |
| 3 | integration-testing | [Integration testing](/t/7205)                    |
| 2 | tomb-package | [`tomb`](/t/11681)                                |
| 2 | worker | [Worker](/t/6561)                                 |
| 2 | worker-interface | [Worker (interface)](/t/11723)                    |
| 2 | worker-package | [Worker (package)](/t/11682)                      |
| | | Agent introspection juju_machine_lock log |
| | logfile-varlogjujumachine-locklog | [Logfile: /var/log/juju/machine-lock.log](/t/112) |
| | | Unit testing |
| | unit-test-suite | [Unit test suite](/t/7209)                        |
| | util-suite | [Util suite](/t/7241)                             |
| | checker | [Checker](/t/7211)                                |
| | integration-test-suite | [Integration test suite](/t/7258)                 |
| | test-includes | [Test includes](/t/7206)                          |
| | | |

[/details]

## Redirects

[details=Mapping table]
| Path | Location |
| -- | -- |
[/details]