
(set-up-your-deployment)=
# Set up your Juju deployment

To set up a cloud deployment with Juju, you need a cloud, Juju, and charms.

1. {ref}`Install the juju CLI client <install-juju>`.

2. Consult our {ref}`list of supported clouds <list-of-supported-clouds>` and prepare your cloud(s).

3. Add your {ref}`cloud definition(s) <add-a-cloud>` and {ref}`cloud credential(s) <add-a-credential>` to Juju and use your `juju` CLI client to {ref}`bootstrap a Juju controller <bootstrap-a-controller>` (control plane) into your cloud. Once the controller is up, you may connect further clouds directly to it.

4. Add {ref}`users <add-a-user>`, {ref}`SSH keys <add-an-ssh-key>`, {ref}`secret backends <add-a-secret-backend>`, etc.

5. Add {ref}`models <add-a-model>` (workspaces) to your controller, then start {ref}`deploying, configuring, integrating, scaling, etc., charmed applications <manage-applications>`. Juju takes care of the underlying infrastructure for you, but if you wish you can also customize {ref}`storage <add-storage>`, {ref}`networking <add-a-space>`, etc.