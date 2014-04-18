#!/usr/bin/env python

"""A small example showing how to write (put) an object to
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
mpath = '/%s/public/hello.txt' % os.environ['MANTA_USER']
content = 'Hello, Manta from python-manta client!'

# To add an object/file to Manta we use the PutObject API endpoint
# (http://apidocs.joyent.com/manta/manta/#PutObject). This corresponds to the
# `put_object` method on the Python Manta client. `put_object` allows you
# to pass in string content, a local file path, or a file-like object.
print('# RawMantaClient.put_object(%r, ...)' % mpath)
client.put_object(mpath, content=content, content_type='text/plain')

print 'Added "%s". This should now be visible at:\n\t%s%s' % (
    mpath, os.environ['MANTA_URL'], mpath)

