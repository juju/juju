---
myst:
  html_meta:
    description: "Juju bundles reference: multi-charm solutions declared as YAML -- bundle declaration (the file, regular and overlay), persistence (none, by design), execution (the deploy expansion, watchers), and rules."
---

(bundle)=
# Bundle
```{audience} user
```

In Juju, a **bundle** is a collection of {ref}`charms <charm>` which have been carefully combined and configured in order to automate a multi-charm solution.

For example, a bundle may include the `wordpress` charm, the `mysql` charm, and the relation between them.

The operations are transparent to Juju and so the deployment can continue to be managed by Juju as if everything was performed manually (what you see in `juju status` is {ref}`applications <application>`, {ref}`relations <relation>`, etc. -- not the bundle entity, but its contents; those records, in the {ref}`model <model>`, are what persists).

(the-bundles-declaration)=
## Bundles in the declaration layer

A bundle is nothing but declaration material: you write one as a
YAML file that states the applications, their configuration, their
relations and their machines, and you deploy it with one of Juju's
clients -- deploying requires {ref}`model write access
<user-access-model-write>`; an overlay customises it at deploy
time, applied the same way.

```{ibnote}
See also: {ref}`Juju | Manage charms <manage-charms>`
```

(types-of-bundle)=
### Types of bundle

The distinction below is not a stored type -- a bundle has no record
(see {ref}`the persistence layer <the-bundles-persistence>`) -- it is
about how the file is used at deploy time.

Whether regular or overlay, a bundle is fundamentally just a YAML file that contains all the applications, configurations, relations, etc., that you want your deployment to have. The two kinds are exclusive by construction -- a file is either deployed as the base or passed as an overlay.

- An **overlay bundle** is a local bundle you pass to `juju deploy <charm/bundle>` via `--overlay <overlay bundle name>.yaml` if you want to customise an upstream charm / bundle (usually the latter, also known as a **base bundle**) for your own needs without modifying the existing charm / bundle directly. For example, you may wish to add extra applications, set custom machine constraints or modify the number of units being deployed. They are especially useful for keeping configuration local, while being able to make use of public bundles. It is also necessary in cases where certain bundle properties (e.g. offers, exposed endpoints) are deployment specific and can _only_ be provided by the bundle's user.
- A **regular bundle** is any bundle that is not an overlay.

(the-bundles-persistence)=
## Bundles in the persistence layer

Nothing here, by design. A bundle is not a record: Juju does not
persist the bundle YAML. Deploying a bundle expands it into ordinary
operations -- charms added, applications deployed, machines
requested, relations joined -- and from then on the model holds only
the results, which are the only records that persist (see
{ref}`the full spine <data-model-full-spine>`). There is no bundle
table to update, no bundle record to remove: destroying the
deployment means destroying the applications it created. And no
record means no states: the states that matter (the applications',
the machines') belong to the entities the bundle expands into.

(the-bundles-execution)=
## Bundles in the execution layer

By the time the command returns, the bundle itself has vanished: what
exists is the applications, machines and relations it expanded into.
A bundle has no machinery of its own; the one execution it has is the
deploy, and even that is a client-side expansion:

```{ggarch}
:file: ../juju.ggarch
:sequence: Bundle deploy
:no-legend:
:caption: Sequence diagram: juju deploy <bundle> --overlay reads the bundle as a YAML multidoc (first document = base, the rest = overlays: relations append, machines overwrite, an empty overlay application REMOVES the base app), snapshots the model status, builds the change graph and topologically sorts it, then applies each change in order (addCharm, deploy, addMachines, addRelation, addUnit, expose, setOptions, create/consume offers). Any error aborts the whole apply.
:alt: User calls juju deploy; client merges overlay into base; controller returns model status snapshot; client builds the change graph and applies changes in order.
```

Deploying is the bundle's one operation, and it is a client-side
expansion: the bundle YAML (base plus overlays merged) is turned into
a dependency-ordered change graph -- charms to add, applications to
deploy, machines to request, relations to join, endpoints to expose,
offers to create or consume -- and each change is replayed through
the ordinary operations. Any error aborts the whole apply. Exporting
a deployed model back to a bundle is not implemented on Juju 4.0.

(the-bundle-watchers)=
### Bundle watchers

Not applicable -- with no bundle record there is nothing to watch;
the entities a bundle created have their own
{ref}`watchers <the-application-watchers>`.

(the-bundle-rules-and-errors)=
## Bundle rules and errors

The bundle has no record, so it has no record rules: the only
enforcement surface is the deploy-time verifier in the deployment
domain, which reads the YAML before anything is expanded. The rules
the **bundle YAML** must satisfy:

- a bundle is a YAML multidoc: the first document is the base, the
  rest are overlays -- relations append, machines overwrite, and an
  overlay application with no properties removes the base's
  application;
- the bundle's series (for example, `bundle: kubernetes`) becomes the
  applications' series.

The errors that encode them (domain/deployment/charm/bundledata.go):

- `bundle has an invalid type %q` -- the type field, when present,
  must be `kubernetes`;
- `bundle machines not valid for Kubernetes bundles` -- a Kubernetes
  bundle declares no machines;
- `bundle declares an invalid base %q` -- the default base, when
  present, must parse as a valid base.
