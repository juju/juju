# Juju Functional Tests

## Running Test Scripts 

Some setup is required before running the tests.

### Installing dependencies

Dependencies are installed via a .deb and pip install.

```bash
$ sudo apt-get install make

$ make install-deps
```

### Required Environment Variables

  * **JUJU_HOME**: The directory the test will use to:
     * Find environments.yaml [(see use of environments.yaml below)](#envs)
     * Find credentials.yaml [(see use of credentials.yaml below)](#envs-creds)     
     * Create a **JUJU_DATA** dir for use for the duration of the test. The directory is created in: $JUJU_HOME/juju-homes/.
  * **JUJU_REPOSITORY**: The directory containing the local dummy charms. You can use '\<juju root\>/acceptancetests/repository'.


### Quick run using LXD
To run a test locally with lxd:

  * ```$ mkdir /tmp/test-run```
  * ```$ export JUJU_HOME=/tmp/test-run```
  * ```$ vim $JUJU_HOME/environments.yaml```
    ```yaml
    environments:
        lxd:
            type: lxd
            test-mode: true
            default-series: xenial
    ```
  * ```export JUJU_REPOSITORY=./path/to/acceptancetests/repository```
  * Now you can run the test with:
     * ```$ ./assess_model_migration.py lxd . . .```

### Quick run using AWS

See [(Use of environments.yaml below)](#envs) and [(Use of credentials.yaml below)](#envs-creds) for a full explanation of the files used here.

To run a test using AWS:

  * ```$ mkdir /tmp/test-run```
  * ```$ export JUJU_HOME=/tmp/test-run```
  * ```$ vim $JUJU_HOME/environments.yaml```
    ```yaml
    environments:
        myaws:
            type: ec2
            test-mode: true
            default-series: xenial
            region: us-east-1
    ```
  * ```$ vim $JUJU_HOME/credentials.yaml```
    ```yaml
    credentials:
      aws:
        credentials:
          auth-type: access-key
          access-key: <access key>
          secret-key: <secret key>
    ```
  * ```export JUJU_REPOSITORY=/path/to/acceptancetests/repository```
  * Now you can run the test with:
     * ```$ ./assess_model_migration.py myaws . . .```

### Which juju binary is used?

If no *juju_bin* argument is passed to an *assess* script it will default to using the juju in your **$PATH**.

So, to use a locally build juju binary either:

  * Ensure you binary is in **$PATH** (i.e. export PATH=/home/user/src/Go/bin:$PATH)
  * Or explicitly pass it to the *assess* script: ./assess_something.py lxd  /home/user/src/Go/bin/juju

### Using an existing controller

Some tests have the ability to be run against an already bootstrapped controller (saving the need for the test to do the bootstrapping).
This feature is available via the ```--existing``` argument for an *assess* script. Check the  ```--help``` output to see if the *assess* script you want to run supports this.

Adding this feature to a new test is as easy as passing ```existing=True``` to ```add_basic_testing_arguments``` which will enable the argument in the script.

### Running a test on an existing controller

To iterate quickly on a test it can be useful to bootstrap a controller and run the test against that multiple times.
This example isolates the juju interactions so your system configuration is not touched.

```bash
# Use freshly built juju binaries
export PATH=/home/user/src/Go/bin:$PATH
export JUJU_DATA=/tmp/testing-controller
# The test will still need JUJU_HOME to find it's environment.yaml and credentials.yaml
#  example as per above.
export JUJU_HOME=~/tmp/test-run
mkdir -p $JUJU_DATA

juju bootstrap lxd/localhost testing-feature-x

./assess_feature-x.py lxd --existing testing-feature-x
```

**Note:** If you don't explicitly set **JUJU_DATA** the test will check for an existing path in this order:

  1. **$JUJU_DATA**
  1. **$XDG_DATA_HOME**/juju
  1. **$HOME**/.local/share/juju
  
### Keeping an environment after a run

Normally a test script will teardown any bootstrapped controllers, if you wish to investigate the environment after a run use ```--keep-env```.  
Using the ```--keep-env``` option will skip any teardown of an environment at the end of a test.

### Use of environments.yaml<a name="envs"></a>

Jujupy test framework uses the *environments.yaml* file found in **JUJU_HOME** to define configuration for a bootstrap-able environment.
The file follows the Juju1 schema for the file of the same name.

The **env** argument to an assess script is a mapping to an environment defined in this file.

For instance if you defined an environment named testing123 of type LXD:

```yaml
environments:
    testing123:
        type: lxd
        test-mode: true
        default-series: xenial
        # You can use config like this too:
        # agent-metadata-url: https://custom.streams.bucket.com
```

You can pass that to an assess script:

```./assess_model_migration.py testing123```

And it will bootstrap using LXD (and won't need a credentials.yaml file, as it's not needed with LXD).

### Use of credentials.yaml<a name="envs-creds"></a>

The Jujupy test framework can use a credentials.yaml file (it looks in $JUJU_HOME) to provide credentials for providers that need it.

The format of the file follows that of Juju1 credentials.yaml, an example:

```yaml
credentials:
  aws:
    credentials:
      auth-type: access-key
      access-key: abc123
      secret-key: abc123
```

# Creating a New CI Test

Test scripts will be run under many conditions to reproduce real cases.
Most scripts cannot assume special knowledge of the substrate, region,
bootstrap constraints, tear down, and log collection, etc.

You can base your new script and its unit tests on the template files.
They provide the infrastructure to setup and tear down a test. Your script
can focus on the unique aspects of your test. Start by making a copy of
template_assess.py.tmpl.

    make new-assess name=my_function

Run make lint early and often. (You may need to do sudo apt-get install python-
flake8).

If your tests require new charms, please write them in Python.
