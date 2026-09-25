---
myst:
  html_meta:
    description: "Juju bundles reference: multi-charm solutions with automated deployment, configuration, and relations. Overlay and regular bundles, deploy mechanics, and rules."
---

(bundle)=
# Bundle

```{ibnote}
See also: {ref}`manage-charms`
```

In Juju, a **bundle** is a collection of {ref}`charms <charm>` which have been carefully combined and configured in order to automate a multi-charm solution.

For example, a bundle may include the `wordpress` charm, the `mysql` charm, and the relation between them.

The operations are transparent to Juju and so the deployment can continue to be managed by Juju as if everything was performed manually (what you see in `juju status` is applications, relations, etc.; that is, not the bundle entity, but its contents).

(the-bundles-records)=
## The bundle's records

(the-bundle-record)=
### The bundle's identity

A bundle is not a record: it is a YAML artifact, and Juju does not
persist it. Deploying a bundle expands it into ordinary operations --
charms added, applications deployed, machines requested, relations
joined -- and from then on the model holds only the results. There is
no bundle table to update, no bundle record to remove: destroying the
deployment means destroying the applications it created.

(the-bundle-in-the-data-model)=
### The bundle in the data model

Nothing in the model database belongs to a bundle: the expansion
writes ordinary charm, application, machine, relation and offer
records (see {ref}`the full spine <data-model-full-spine>`). The one
place a bundle exists as a unit is the deploy machinery's change
graph, which lives only for the duration of the deploy.

(the-bundle-states)=
### Bundle states

Not applicable -- there is no bundle record, hence no bundle state:
the states that matter (the applications', the machines') belong to
the entities the bundle expands into.

(types-of-bundle)=
### Types of bundle

Whether regular or overlay, a bundle is fundamentally just a YAML file that contains all the applications, configurations, relations, etc., that you want your deployment to have. The two kinds are exclusive by construction -- a file is either deployed as the base or passed as an overlay.

- An **overlay bundle** is a local bundle you pass to `juju deploy <charm/bundle>` via `--overlay <overlay bundle name>.yaml` if you want to customise an upstream charm / bundle (usually the latter, also known as a **base bundle**) for your own needs without modifying the existing charm / bundle directly. For example, you may wish to add extra applications, set custom machine constraints or modify the number of units being deployed. They are especially useful for keeping configuration local, while being able to make use of public bundles. It is also necessary in cases where certain bundle properties (e.g. offers, exposed endpoints) are deployment specific and can _only_ be provided by the bundle's user.
- A **regular bundle** is any bundle that is not an overlay.

(the-bundles-machinery)=
## The bundle's machinery

A bundle has no machinery of its own: it is a client-side YAML
artifact; deploying it is a client-side expansion, and from then on
the model holds only the results (which have their own machinery).

(the-bundle-operations)=
### Bundle operations

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

The rules the **bundle YAML** must satisfy:

- a bundle is a YAML multidoc: the first document is the base, the
  rest are overlays -- relations append, machines overwrite, and an
  overlay application with no properties removes the base's
  application;
- the bundle's series (for example, `bundle: kubernetes`) becomes the
  applications' series;
- the expansion is all-or-nothing: any change that fails aborts the
  whole apply, leaving the model as it was.

(related-entities-bundle)=
## Entities related to the bundle

- **Charms** are what a bundle deploys (see {ref}`charm <charm>`).
- **Applications, machines, relations and offers** are what it
  expands into -- the only records that persist
  (see {ref}`application <application>`, {ref}`machine <machine>`,
  {ref}`relation <relation>`, {ref}`offer <offer>`).
- **The model** is where the results live (see {ref}`model <model>`).
