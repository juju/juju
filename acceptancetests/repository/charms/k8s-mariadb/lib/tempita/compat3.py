import sys

__all__ = ['b', 'basestring_', 'bytes', 'next', 'is_unicode']

if sys.version < "3":
    b = bytes = str
    basestring_ = str
else:

    def b(s):
        if isinstance(s, str):
            return s.encode('latin1')
        return bytes(s)
    basestring_ = (bytes, str)
    bytes = bytes
text = str

if sys.version < "3":

    def next(obj):
        return obj.__next__()
else:
    next = next

if sys.version < "3":

    def is_unicode(obj):
        return isinstance(obj, str)
else:

    def is_unicode(obj):
        return isinstance(obj, str)


def coerce_text(v):
    if not isinstance(v, basestring_):
        if sys.version < "3":
            attr = '__unicode__'
        else:
            attr = '__str__'
        if hasattr(v, attr):
            return str(v)
        else:
            return bytes(v)
    return v
