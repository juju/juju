(bundle)=
# Bundle

> See also: {ref}`manage-charms`

In Juju, a **bundle** is a collection of {ref}`charms <charm>` which have been carefully combined and configured in order to automate a multi-charm solution.

For example, a bundle may include the `wordpress` charm, the `mysql` charm, and the relation between them.

The operations are transparent to Juju and so the deployment can continue to be managed by Juju as if everything was performed manually (what you see in `juju status` is applications, relations, etc.; that is, not the bundle entity, but its contents).

Bundles can be of two kinds, **overlay** and **regular**.

- An **overlay bundle** is a local bundle you pass to `juju deploy <charm/bundle>` via `--overlay <overlay bundle name>.yaml` if you want to customise an upstream charm / bundle (usually the latter, also known as a **base bundle**) for your own needs without modifying the existing charm / bundle directly. For example, you may wish to add extra applications, set custom machine constraints or modify the number of units being deployed. They are especially useful for keeping configuration local, while being able to make use of public bundles. It is also necessary in cases where certain bundle properties (e.g. offers, exposed endpoints) are deployment specific and can _only_ be provided by the bundle's user.
- A **regular bundle** is any bundle that is not an overlay.


Whether regular or overlay, a bundle is fundamentally just a YAML file that contains all the applications, configurations, relations, etc., that you want your deployment to have.


