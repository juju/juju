from charmhelpers.core import hookenv

import charms.reactive


class NullTracer(object):
    """
    NullTracer is the default tracer and tracer base class, and does nothing

    This API will need to be adapted when charms.reactive internals
    change and cannot be considered stable.
    """
    def start_dispatch(self):
        """
        Start of handler dispatch
        """
        pass

    def start_dispatch_phase(self, phase, handlers):
        """
        Start of a particular dispatch phase. ie. hooks, restricted, or other.
        """
        pass

    def start_dispatch_iteration(self, iteration, handlers):
        """
        Start of an iteration of the dispatch phase.

        Only the 'other' phase (the main reactive loop) has multiple
        phases, and is the only phase that invokes this method.
        """

    def set_flag(self, flag):
        """
        A charms.reactive flag is being set.
        """
        pass

    def clear_flag(self, flag):
        """
        A charms.reactive flag is being cleared.
        """
        pass


class LogTracer(NullTracer):
    """
    LogTracer logs flags and handler activation to the Juju charm log

    Expect formatting and verbosity to change in future releases.
    """
    LEVEL = hookenv.DEBUG

    def __init__(self):
        self._active_handlers = set()
        self._msgs = []

    def start_dispatch(self):
        all_flags = charms.reactive.get_flags()
        self._emit("starting handler dispatch, {} flags set".format(len(all_flags)))
        for f in all_flags:
            self._emit("set flag {}".format(f))
        self._flush()

    def start_dispatch_phase(self, phase, handlers):
        self._active_handlers = set(handlers)
        if phase == 'other':
            self._emit("main dispatch loop, {} handlers queued".format(len(handlers)))
        else:
            self._emit("{} phase, {} handlers queued".format(phase, len(handlers)))
        for h in sorted(h.id() for h in handlers):
            self._emit("++   queue handler {}".format(h))
        self._flush()

    def start_dispatch_iteration(self, iteration, handlers):
        self._active_handlers = set(handlers)

    def set_flag(self, flag):
        self._flag("set flag {}".format(flag))

    def clear_flag(self, flag):
        self._flag("cleared flag {}".format(flag))

    def _emit(self, msg):
        self._msgs.append("tracer: {}".format(msg))

    def _flush(self):
        if self._msgs:
            if len(self._msgs) > 1:
                self._msgs.insert(0, "tracer>")
            hookenv.log("\n".join(self._msgs), self.LEVEL)
            self._msgs = []

    def _flag(self, msg):
        self._emit(msg)
        prev_handlers = self._active_handlers
        next_handlers = set(h for h in charms.reactive.bus.Handler.get_handlers() if h.test())

        for h in sorted(h.id() for h in (next_handlers - prev_handlers)):
            self._emit("++   queue handler {}".format(h))

        for h in sorted(h.id() for h in (prev_handlers - next_handlers)):
            self._emit("-- dequeue handler {}".format(h))

        self._flush()

        self._active_handlers = next_handlers


_tracer = None


def install_tracer(tracer):
    global _tracer
    _tracer = tracer
    # Disable tracing when we hit atexit, to avoid spam from layers
    # such as when the base layer tears down its automatic flags.
    hookenv.atexit(install_tracer, NullTracer())


def tracer():
    global _tracer
    return _tracer


install_tracer(NullTracer())
