# Copyright 2012 Joyent, Inc.  All rights reserved.

"""python-manta errors"""

__all__ = ["MantaError", "MantaResourceNotFoundError", "MantaAPIError"]

import logging
import json



#---- globals

log = logging.getLogger('manta.errors')



#---- exports

class MantaError(Exception):
    pass

class MantaResourceNotFoundError(Exception):
    pass

class MantaAPIError(MantaError):
    """An errors from the Manta API.

    @param res {httplib2 Response}
    @param content {str} The raw response content.
    """
    def __init__(self, res, content):
        self.res = res
        if res['content-type'] == 'application/json':
            self.body = json.loads(content)
            self.code = self.body["code"]
            message = "(%(code)s) %(message)s" % self.body
        else:
            self.body = message = content
        MantaError.__init__(self, message)
