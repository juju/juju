# Security policy

## Supported versions

Security updates will be released for versions that [receive security updates](https://juju.is/docs/juju/roadmap).

## Reporting a vulnerability

Please provide a description of the issue, the steps you took to
create the issue, affected versions, and, if known, mitigations for
the issue.

The preferred way to report a security issue is through
[GitHub's security advisory for this project](https://github.com/juju/juju/security/advisories/new). The advisory is private
by default; repo admins review it. See
[Privately reporting a security
vulnerability](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing/privately-reporting-a-security-vulnerability)
for instructions on reporting using GitHub's security advisory feature.

The [Ubuntu Security disclosure and embargo
policy](https://ubuntu.com/security/disclosure-policy) contains more
information about how can contact us, what you can expect when you contact us,
and what we expect from you.

## The CVE process

In software, a CVE (common vulnerability and exposure) is a security issue
that meets certain standard identifiers (see more: the
[CVE website](https://www.cve.org/)). In Juju, once a security advisory has
been filed as described above, the process is as follows:

If the advisory is confirmed as a CVE, all the usual CVE protocols apply: it
gets assigned a CVE number; an embargo is set in place; and a countdown
starts for when the CVE must be made public. This also triggers a countdown
for when, ideally, a fix must be released.

When the fix is ready, its release must be prepared privately, that is, from
a private branch. The process is as described in the GitHub documentation
linked above, with the following mention about solutions QA:

- If the timing is such that it doesn't align with the normal release
  cadence, the private branch is created from the latest release tag and the
  fix is added to that. Because there's low risk of regression, the candidate
  does not go through the usual Solutions QA verification. CI tests plus
  manual verification are deemed sufficient for release.
- If the timing is such that it aligns with the normal release cadence, the
  fix is released as part of the normal release process (aside from being
  built from a private branch) and goes through Solutions QA.

Once the fix has been released, the embargo is lifted and the security
advisory is published on
[GitHub](https://github.com/juju/juju/security/advisories) with the related
CVE record published on the [CVE website](https://www.cve.org/). Users are
made aware of the issue, the fix, and what they need to do to get the fix
through the [Roadmap & Releases](https://juju.is/docs/juju/roadmap) which
also point to the official CVE record on the [CVE
website](https://www.cve.org/).

