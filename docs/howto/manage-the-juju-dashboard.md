(manage-the-juju-dashboard)=
# How to manage the Juju dashboard

## Set up the dashboard

The Juju dashboard is set up automatically upon controller bootstrap.

## Access the dashboard

First, use the `dashboard` command to get the IP address and login credentials to access the dashboard. This command will need to be run on the same machine as your browser as it will proxy a secure connection to the controller.

```text
juju dashboard
```

This will produce output similar to the following:

```text
Dashboard for controller "my-controller" is enabled at:

https://10.55.60.10:17070/dashboard

Your login credential is:

username: admin

password: 1d191f0ef257a3fc3af6be0814f6f1b0
```

Now copy-paste the URL into the browser to access the dashboard.

```{important}

If you don't want to copy and paste the URL manually, typing `juju dashboard --browser` will open the link in your default browser automatically.

```

Your browser will give you an error message when you open the URL warning that the site certificate should not be trusted. This is because Juju is generating a self-signed SSL certificate rather than one from a certificate authority (CA). Depending on your browser, choose to manually proceed or add an exception to continue past the browser's error page.

After opening the Juju dashboard URL, you are greeted with the login window, where you will have to provide the credentials to access the model. These credentials can be copied from the output of `juju dashboard`.

If you'd rather not have your login credentials displayed in the output of `juju dashboard`, they can be suppressed by adding the `--hide-credential` argument.


> See more: {ref}`command-juju-dashboard`