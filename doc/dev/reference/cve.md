[CVE Website]: https://www.cve.org/

[GitHub]: https://github.com/juju/juju/security/advisories

[Roadmap & Releases]: https://juju.is/docs/juju/roadmap

In software, a CVE (common vulnerability and exposure) is a security issue that meets certain standard identifiers (see
more: [CVE Website]). In Juju, the process around CVEs is as follows:

People noticing a potential vulnerability in the Juju codebase create a security advisory
(see https://github.com/juju/juju/security/advisories/new).

The advisory is by default private. Repo admins review it. If it’s confirmed as a CVE, all the usual CVE protocols
apply: It gets assigned a CVE number; an embargo is set in place; and a countdown starts for when the CVE must be made
public. This also triggers a countdown for when, ideally, a fix must be released.

When the fix is ready, its release must be prepared privately, that is, from a private branch. The process is as
described
in [Privately reporting a security vulnerability - GitHub Docs](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability),
with the following mention about solutions QA:

- If the timing is such that it doesn’t align with the normal release cadence, the private branch is created from the
  latest release tag and the fix is added to that. Because there’s low risk of regression, the candidate does not go
  through the usual Solutions QA verification. CI tests plus manual verification are deemed sufficient for release.
- If the timing is such that it aligns with the normal release cadence, the fix is released as part of the normal
  release process (aside from being built from a private branch) and goes through Solutions QA.

Once the fix has been released, the embargo is lifted and the security advisory is published on [GitHub]
with the related CVE record published on [CVE Website]. Users are made aware of the issue, the fix, and what they need
to do to get the fix through the release notes (e.g., [Roadmap & Releases]) which also point to the official CVE record
on [CVE Website].