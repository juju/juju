#!/usr/bin/env python

"""A small example showing how to list a directory in
Manta using the python-manta client.
"""

import os
from pprint import pprint
import sys

# Import the local manta module.
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
import manta


def get_client():
    MANTA_USER = os.environ['MANTA_USER']
    MANTA_URL = os.environ['MANTA_URL']
    MANTA_TLS_INSECURE = bool(os.environ.get('MANTA_TLS_INSECURE', False))
    MANTA_NO_AUTH = os.environ.get('MANTA_NO_AUTH', 'false') == 'true'
    if MANTA_NO_AUTH:
        signer = None
    else:
        MANTA_KEY_ID = os.environ['MANTA_KEY_ID']
        signer = manta.SSHAgentSigner(key_id=MANTA_KEY_ID)
    client = manta.MantaClient(url=MANTA_URL,
        account=MANTA_USER,
        signer=signer,
        # Uncomment this for verbose client output for test run.
        #verbose=True,
        disable_ssl_certificate_validation=MANTA_TLS_INSECURE)
    return client


#---- mainline

client = get_client()
mpath = '/%s/public' % os.environ['MANTA_USER']

# First, there is the base `RawMantaClient` that just provides a light wrapper
# around the Manta REST API (http://apidocs.joyent.com/manta/api.html).
# For example, the `list_directory` method corresponds to the ListDirectory
# endpoint (http://apidocs.joyent.com/manta/api.html#ListDirectory).
print('# RawMantaClient.list_directory(%r)' % mpath)
d = client.list_directory(mpath)
pprint(d)


# Then there are some convenience methods on the `MantaClient` subclass. For
# example the `ls` method will handle paging through >1000 entries in a
# directory. Note that this returns a dict of name to dirent.
print('')
print('# MantaClient.ls(%r)' % mpath)
d = client.ls(mpath)
pprint(d)

