---
myst:
  html_meta:
    description: "Add, discover and inspect cloud credentials with the Juju CLI, change a model's credential, update shared material, and remove unused copies."
---

(manage-credentials)=
# How to manage credentials

```{ibnote}
See also: {ref}`Credential <credential>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

Choose a {ref}`credential destination <credential-stores-and-defaults>` before running a command that writes to either store. For the controller examples, {ref}`sign in <command-juju-login>` and substitute your controller's name for `production`.

(add-a-credential)=
## Add a credential

```{ibnote}
See also: {ref}`Registering a credential <registering-a-credential>`, [Terraform Provider for Juju | Define a credential](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-credentials/#define-a-credential)
```

To use a credential on a controller, make sure the {ref}`cloud is registered there <add-a-cloud>`. Choose a new name unless you intend to replace an existing controller credential: other models may already use that name under your account. Use a name starting with a letter, such as `production-deploy`.

(add-a-machine-cloud-credential)=
(add-a-credential-for-a-machine-cloud)=
### Add a credential for a machine cloud

```{ibnote}
See also: {ref}`Credential definition <credential-definition>`, {ref}`Supported clouds <list-of-supported-clouds>`
```

Choose an authentication type supported by the cloud and obtain the corresponding material from the provider. For example, an {ref}`EC2 access-key credential <ec2-credential-access-key>` needs an access key and a secret key. For local LXD, first check {ref}`how Juju obtains the local credential <lxd-credential>`; this depends on access to LXD on the client machine, not on a Juju controller grant.

**Enter a credential interactively.** To store a credential on both this client and the `production` controller, run:

```text
juju add-credential aws --client -c production
```

Enter the credential name and choose the region and authentication type when prompted. Supply the attributes requested for that type. Check the separate client and controller messages: a local save alone does not make the credential available to a model.

```{ibnote}
See more: {ref}`command-juju-add-credential`
```

**Add a credential from a file.** Prepare a private YAML file. Under `credentials`, use the cloud name, then the credential name, then `auth-type` and its attributes. For example, if you use an {ref}`EC2 instance role <ec2-credential-instance-role>` with an existing instance profile named `juju-production`, the input is:

```yaml
credentials:
  aws:
    production-deploy:
      auth-type: instance-role
      instance-profile-name: juju-production
```

To add the entries for `aws` from `credentials.yaml` to the client only:

```text
juju add-credential aws -f credentials.yaml --client
```

A file can contain several named entries. If a name already exists locally, file-based add reports it and skips that local copy; other entries can still be saved. If you also selected a controller, a local duplicate can still be submitted there. To replace a local copy deliberately, {ref}`update it <update-a-credential>`.

```{ibnote}
See more: {ref}`command-juju-add-credential`
```

**Discover a credential from the provider's local configuration.** To scan only the EC2 provider, use the provider type `ec2`. Omit it to scan all available provider types. An existing name can be overwritten in the selected store, so check the name and cloud before choosing an entry.

```text
juju autoload-credentials ec2 --client
```

Select an entry by number and choose a compatible cloud. Enter `q` when you have finished. If you selected a controller as a destination, Juju then attempts to upload the selections for clouds available there. `No cloud credentials found.` means the scan found no candidates.

```{ibnote}
See more: {ref}`command-juju-autoload-credentials`
```

(add-a-kubernetes-cloud-credential)=
(add-a-credential-for-a-kubernetes-cloud)=
### Add a credential for a Kubernetes cloud

```{ibnote}
See also: {ref}`Add a Kubernetes cloud <add-a-kubernetes-cloud>`
```

To register a Kubernetes cluster and its credential, select a kubeconfig context that lets Juju prepare an administrative credential in the intended cluster. Use `--client` for this client and add `-c production` to also register them on that controller. For a cluster without workload storage, use `--skip-storage` to skip the cluster metadata and storage lookup. Juju still prepares the credential.

Register the `staging` context as `staging-k8s` on this client, then inspect the new credential, which has the same name as the cloud.

```text
juju add-k8s staging-k8s --context-name staging --client
juju show-credential staging-k8s staging-k8s --client
```

```{ibnote}
See more: {ref}`command-juju-add-k8s`, {ref}`command-juju-show-credential`
```

(view-all-the-known-credentials)=
(list-credential-names)=
## List credential names

```{ibnote}
See also: {ref}`Client vs. controller credential <client-vs-controller-credential>`, {ref}`Inspecting your credential contents <inspecting-your-credential-contents>`
```

To list the names on this client and in your account on the current controller, run:

```text
juju credentials
```

The results are grouped by store and by cloud. For example:

```text
Controller Credentials:
Cloud  Credentials
aws    production-deploy

Client Credentials:
Cloud  Credentials
aws    production-deploy*, staging-deploy
```

The asterisk marks a **local** default. To list only your records on a particular controller, run `juju credentials -c production`. To narrow the client list to one cloud, run `juju credentials aws --client`. A positional cloud name does not narrow the controller results.

```{ibnote}
See more: {ref}`command-juju-credentials`
```

(view-details-about-a-credential)=
## View details about a credential

```{ibnote}
See also: {ref}`Inspecting your credential contents <inspecting-your-credential-contents>`
```

To inspect one credential, provide **both** its cloud and its name. For example, to read your controller copy:

```text
juju show-credential aws production-deploy -c production
```

Look under `controller-credentials` for its authentication type, `validity-check`, and any listed models and access levels. `valid` is a {ref}`stored state <credential-validity>`, not a new test against the provider. The model list can omit a dependent model you cannot access. For the complete set of your credential contents on that controller, omit both positional arguments. If nothing can be displayed, check any lookup message and the selected store before concluding a credential is missing.

To compare the controller copy with a local copy, add `--client` to the same command. The output is YAML. A local copy does not include the controller's validity or model associations.

```{warning}
Hidden values are omitted by default. Use `--show-secrets` only when you need the values and can keep the terminal, logs and any output file private. Inspect one named credential rather than disclosing every entry. Inspection output is not an input file for adding credentials.
```

```{ibnote}
See more: {ref}`command-juju-show-credential`
```

(set-or-get-the-default-credential)=
## Set or get the default credential

```{ibnote}
See also: {ref}`Credential stores and defaults <credential-stores-and-defaults>`
```

To select an existing local credential for future operations on `aws`, then read back the preference:

```text
juju default-credential aws production-deploy
juju default-credential aws
```

To clear the preference without deleting the credential, run `juju default-credential aws --reset`. Changing the preference on this client does not switch a model or set a controller-wide default.

```{ibnote}
See more: {ref}`command-juju-default-credential`
```

(add-a-credential-to-a-model)=
(change-the-credential-a-model-uses)=
## Change the credential a model uses

```{ibnote}
See also: {ref}`Changing a model's credential <changing-a-models-credential>`, [Terraform Provider for Juju | Add a credential to a model](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-credentials/#add-a-credential-to-a-model)
```

To switch one model, use an account {ref}`authorised to change that model's credential <changing-a-models-credential>` and check that model changes are enabled. Choose a separately named credential under that account for the model's cloud, or {ref}`add one <add-a-credential>`. Confirm it can access the model's existing resources.

```{warning}
Check the controller and client copies before switching. If Juju cannot find the controller copy **or the lookup fails**, it tries to upload the local copy before requesting the switch. A failed lookup does not mean the name is unused: an upload under the same cloud, owner and name replaces the material for **every model** using that credential, even if the controller then refuses the switch. If you cannot verify which credential the name identifies, resolve the lookup problem before running the command.
```

For example, switch the AWS model `trinity` to `trinity-deploy` and inspect the model:

```text
juju set-credential -m trinity aws trinity-deploy
juju show-model trinity
```

Check the name, owner and cloud under `credential`. Other models keep their existing bindings. If Juju uploaded a copy before a refused switch, {ref}`inspect the uploaded controller credential <view-details-about-a-credential>` before retrying.

```{ibnote}
See more: {ref}`command-juju-set-credential`, {ref}`command-juju-show-model`
```

(update-a-credential)=
## Update a credential

```{ibnote}
See also: {ref}`Updating a credential <updating-a-credential>`, [Terraform Provider for Juju | Update a credential](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-credentials/#update-a-credential)
```

Before rotating shared material, {ref}`inspect the controller credential <view-details-about-a-credential>` and confirm that the replacement is appropriate for **every** model using it. Supply the complete set of attributes: an update replaces the previous contents, not just the fields you name. If only one model needs different access, {ref}`switch that model <change-the-credential-a-model-uses>` instead.

**Update from a file.** Name the cloud and credential to update one entry. Omitting the credential name selects all entries for that cloud. Omitting both names selects entries across the file. Check the selected entries and destination before running the command. For example, replace the controller's `production-deploy` credential with material from a private file.

```text
juju update-credential aws production-deploy -f credentials.yaml -c production
```

Review the reported results before relying on the new material.

```{ibnote}
See more: {ref}`command-juju-update-credential`
```

**Update from the client copy.** If the local copy already holds the new material, upload it to the controller:

```text
juju update-credential aws production-deploy -c production
juju show-credential aws production-deploy -c production
```

The first command reads the cache; it is not an interactive credential editor. Review the credential and model messages, then verify that the required cloud operation works. A successful update does not establish that the provider grants the access your models need.

```{ibnote}
See more: {ref}`command-juju-update-credential`, {ref}`command-juju-show-credential`
```

(remove-a-credential)=
## Remove a credential

```{ibnote}
See also: {ref}`Revoking a credential <revoking-a-credential>`, [Terraform Provider for Juju | Remove a credential](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-credentials/#remove-a-credential)
```

To retire one of your controller credentials, {ref}`inspect it <view-details-about-a-credential>` and arrange for administrators to {ref}`switch dependent models <change-the-credential-a-model-uses>` to a replacement. Inspection can omit models you cannot access. Sign in as the credential owner. If ordinary removal reports that the credential is `still used by` models, keep the old material usable while the remaining models switch, then retry.

```text
juju remove-credential aws production-deploy -c production
```

```{ibnote}
See more: {ref}`command-juju-remove-credential`
```

To remove the independent copy on this client as well, run:

```text
juju remove-credential aws production-deploy --client
```

This also clears the local preference if it named the removed credential. To invalidate the underlying key or token in the cloud account, revoke it with the provider once its consumers have switched.

```{ibnote}
See more: {ref}`command-juju-remove-credential`
```
