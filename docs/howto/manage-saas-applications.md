---
myst:
  html_meta:
    description: "Inspect, rename, and remove SAAS applications in a consuming Juju model."
---

(manage-saas-applications)=
# How to manage SAAS applications

```{ibnote}
See also: {ref}`saas-application`
```

The first time you {ref}`consume <consume-an-offer>` an {ref}`offer <offer>` in a {ref}`model <model>`, Juju creates a {ref}`SAAS application <saas-application>` there. Other {ref}`applications <application>` in the consuming model can use that same SAAS application.

(view-a-saas-application)=
## Inspect a SAAS application

```{ibnote}
See also: {ref}`SAAS application status <saas-application-status>`
```

To inspect a SAAS application's endpoints and status, run `juju status --format yaml` in the consuming model and look up its local name under `application-endpoints`. The entry shows the offer URL, the offered endpoints, and the application status, including a message when one is available. For example:

```text
juju status --format yaml
...
application-endpoints:
  hosted-mysql:
    url: prod:admin/default.hosted-mysql
    endpoints:
      database:
        interface: mysql_client
        role: provider
    application-status:
      current: active
...
```

```{ibnote}
See more: {ref}`command-juju-status`
```

(rename-a-saas-application)=
## Rename a SAAS application

```{ibnote}
See also: {ref}`SAAS application names <saas-application-names>`, {ref}`Consuming an offer <consuming-an-offer>`
```

There is no in-place rename. Replacing the SAAS application removes its existing relations. To use a new local name:

1. Record the offer URL from `juju status --format yaml`. Run `juju status --relations` and note the endpoints of any relations you need to recreate.
2. {ref}`Remove the SAAS application <remove-a-saas-application>`. Wait for removal to finish before reusing the offer.
3. Consume the same offer under the new name. For example:

   ```text
   juju consume prod:admin/default.hosted-mysql mysql-prod
   ```

4. {ref}`Integrate <integrate-with-an-offer>` each application that still needs the offer, using the new local name. For example:

   ```text
   juju integrate mediawiki:db mysql-prod:database
   ```

```{ibnote}
See more: {ref}`command-juju-consume`, {ref}`command-juju-integrate`
```

(remove-a-saas-application)=
## Remove a SAAS application

```{ibnote}
See also: {ref}`Removing a SAAS application <removing-a-saas-application>`
```

To remove a SAAS application and **all its relations in the consuming model**, without removing the offer from the offering model:

1. Run `juju remove-saas` followed by the local application name, not the offer URL. For example:

   ```text
   juju remove-saas hosted-mysql
   ```

2. In the consuming model, watch the SAAS section refresh:

   ```text
   watch -n 1 juju status
   ```

   When the local name disappears from the SAAS section, press {kbd}`Ctrl`+{kbd}`C` to stop watching. If it was the last SAAS application, the section disappears too.

```{ibnote}
See more: {ref}`command-juju-remove-saas`, {ref}`command-juju-status`
```
