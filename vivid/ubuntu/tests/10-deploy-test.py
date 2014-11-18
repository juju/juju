#!/usr/bin/python3

# This Amulet based tests
# The goal is to ensure the Ubuntu charm
# sucessfully deploys and can be accessed.
# Note the Ubuntu charm does not have any 
# relations or config options.

import amulet
#import os
#import requests

# Timeout value, in seconds to deploy the environment
seconds = 900

# Set up the deployer module to interact and set up the environment.
d = amulet.Deployment()

# Define the environment in terms of charms, their config, and relations.

# Add the Ubuntu charm to the deployment.
d.add('ubuntu')

# Deploy the environment currently defined
try:
    # Wait the defined about amount of time to deploy the environment.
    # Setup makes sure the services are deployed, related, and in a
    # "started" state.
    d.setup(timeout=seconds)
    # Use a sentry to ensure there are no remaining hooks being execute
    # on any of the nodes
##    d.sentry.wait()
except amulet.helpers.TimeoutError:
    # Pending the configuration the test will fail or be skipped
    # if not deployed properly.
    error_message = 'The environment did not deploy in %d seconds.' % seconds
    amulet.raise_status(amulet.SKIP, msg=error_message)
except:
    # Something else has gone wrong, raise the error so we can see it and this
    # will automatically "FAIL" the test.
    raise

# Access the Ubuntu instance to ensure it has been deployed correctly 

# Define the commands to be ran
lsb_release_command = 'cat /etc/lsb-release'
uname_command = 'uname -a'

# Cat the release information
output, code = d.sentry.unit['ubuntu/0'].run(lsb_release_command)
# Confirm the lsb-release command was ran successfully
if (code != 0):
    error_message = 'The ' + lsb_release_command + ' did not return the expected return code of 0.'
    print(output)
    amulet.raise_status(amulet.FAIL, msg=error_message)
else:
    message = 'The lsb-release command successfully executed.'
    print(output)
    print(message)

# Get the uname -a output 
output, code = d.sentry.unit['ubuntu/0'].run(uname_command)
# Confirm the uname command was ran successfully
if (code != 0):
    error_message = 'The ' + uname_command + ' did not return the expected return code of 0.'
    print(output)
    amulet.raise_status(amulet.FAIL, msg=error_message)
else:
    message = 'The uname command successfully executed.'
    print(output)
    print(message)
