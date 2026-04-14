(juju405)=
# Juju 4.0.5
🗓️ 7 Apr 2026

This is a critical security and bug fix release for Juju 4.0.

## 🛠️ Fixes

### CVE: Certificates
The Dqlite cluster endpoint on the Juju controller does not verify certificates correctly.

Because the server does not validate the client certificate, an attacker that can reach this endpoint may be able to
join the Dqlite cluster as a member. From there, they could read or change all cluster data, including actions such as
privilege escalation or opening firewall ports.

The client also does not verify the server certificate, which makes man-in-the-middle attacks possible. In practice,
both sides trust unauthenticated peers, so the connection cannot be considered secure.

We now ensure proper verification:

* fix [CVE-2026-4370](https://github.com/juju/juju/security/advisories/GHSA-gvrj-cjch-728p#top)

**What you need to do:**

- Option 1 (recommended): Upgrade your controller immediately from `4.0.x` to `4.0.5`.
- Option 2: Disable HA (High Availability) on the controller. If HA is not strictly needed in your environment, running
a single controller removes the need for Dqlite replication.
- Option 3: Limit access to port 17666. Add firewall rules to deny all inbound traffic to this port except from Juju
controller IP addresses. Only controller nodes should be allowed to connect to it.