FROM public.ecr.aws/ubuntu/ubuntu:24.04
ARG TARGETOS
ARG TARGETARCH

# juju-dynamic-image-layout: the jujud controller binary is dynamically linked.
# The build stages its native runtime library closure at
# ${TARGETOS}_${TARGETARCH}/lib/ and the image owns it under /opt/lib. The
# marker line above is stable: release tooling gates on it, do not remove.

# Add the syslog user for audit logging and juju rootless controller.
# 170 uid/gid must be updated here and in internal/provider/kubernetes/constants/constants.go
RUN useradd --system --no-create-home --shell /usr/sbin/nologin syslog \
    && groupadd --gid 170 juju \
    && useradd --uid 170 --gid 170 --no-create-home --shell /usr/bin/bash juju \
    && apt-get update \
    && apt-get install -y --no-install-recommends vim less ca-certificates \
    && apt-get remove sudo --purge \
    && apt-get autoremove --purge \
    && rm -rf /var/lib/apt/lists/* \
    && rm -rf /root/.cache

WORKDIR /var/lib/juju
COPY ${TARGETOS}_${TARGETARCH}/bin/jujud \
     ${TARGETOS}_${TARGETARCH}/bin/jujuagentd \
     ${TARGETOS}_${TARGETARCH}/bin/jujuc \
     ${TARGETOS}_${TARGETARCH}/bin/containeragent \
     ${TARGETOS}_${TARGETARCH}/bin/pebble /opt/
# juju-dynamic-image-layout: staged jujud native runtime closure.
COPY ${TARGETOS}_${TARGETARCH}/lib/ /opt/lib/
COPY controller-wrappers/ /usr/bin/

# jujud dispatches on argv[0] to run the introspect command when
# invoked as "juju-introspect" (see cmd/jujud/run.go). The CAAS
# controller wrappers rely on this symlink, mirroring the IAAS snap
# which creates the same link in snapcraft.yaml.
RUN ln -s jujud /opt/juju-introspect

# juju-dynamic-image-layout: resolve the jujud native runtime closure from
# the image-owned /opt/lib (the Ubuntu base does not carry libdqlite).
ENV LD_LIBRARY_PATH=/opt/lib

ENTRYPOINT ["sh", "-c"]
