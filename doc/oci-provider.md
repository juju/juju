# OCI provider


## Prerequisites

To bootstrap juju on OCI you will need the following:

 * A valid OCI account. Please refer to https://cloud.oracle.com/cloud-infrastructure for information on how to obtain an account
 * OCI command line client

## Setting up the command line client

The client itself is not strictly needed, but it is the easiest way to set up a public/private keypair, as well as access credentials that can later be ingested by Juju. For installation and configuration instructions for the CLI, please refer to the [OCI documentation page](https://docs.us-phoenix-1.oraclecloud.com/Content/API/SDKDocs/cliinstall.htm).

After successfully going through the configuration process, you should have a folder called ```.oci``` in your ```$HOME``` (on both Linux systems and Windows). You should be able to view its contents by running:

```bash
cat $HOME/.oci/config
```

You should also have a public/private keypair in ```pem``` format in the same folder. If you used the default values for the keynames, those files should be called:

  * oci_api_key.pem - this is your private key. Keep this safe
  * oci_api_key_public.pem - This is your public key

The public key must be uploaded to your OCI account, to enable API access. To upload your key, log into your OCI console, and navigate to ```Identity --> Users --> <your user> --> API keys```. Click on ```Add Public Key```, and simply copy and paste the contents of ```oci_api_key_public.pem```. This will enable you to access the API as the user to which you just associated the public key to.


## Preparing Juju for bootstrap


Now that you have configured access to your OCI account, it's time to configure Juju to use that info. First, we need to add the OCI cloud, so create a file called ```~/oci.yaml``` with the following contents:

```yaml
clouds:
  oci:
    type: oci
    auth-types: [httpsig]
    regions:
      # The region URL is computed by the
      # OCI SDK.
      us-phoenix-1: {}
```

Now add the cloud:

```bash
juju add-cloud oci -f /tmp/oci.yaml
```

And finally, import the credentials for this cloud:

```bash
juju autoload-credentials
```

## Bootstrap Juju

For the moment in order to be able to bootstrap on top of OCI, juju requires that you use an account with administrative permissions. You will also need to create a new compartment. Compartments are a way to organize deployed resources. For information on how to create a new compartment, please refer to the [oracle documentation page](https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Tasks/managingcompartments.htm). After creating the compartment, you will need to make note of the compartment ID.


Time to bootstrap:

```bash
juju bootstrap --config compartment-id=<compartment_id from the above step> oci
```

Add ```--build-agent``` if you are testing.


