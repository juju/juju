FROM public.ecr.aws/ubuntu/ubuntu:24.04
ARG TARGETOS
ARG TARGETARCH

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

# juju-dynamic-image-layout: jujud is linked dynamically and needs the
# shared libraries staged into /opt/lib (see the COPY above), because the
# Ubuntu base image does not ship Dqlite. jujud finds /opt/lib through an
# rpath embedded when it was linked (CGO_RPATH in the Makefile, and the
# matching rpath in the snapcraft.yaml jujud part, so the snap-built binary
# extracted into release images resolves it too), which works no matter how
# the container is started - including the Kubernetes launch paths that
# replace the image command. The LD_LIBRARY_PATH export below is only a
# fallback for bare `docker run <image> <cmd>` calls; with no command it
# runs nothing and exits 0.
ENTRYPOINT ["sh", "-c", "export LD_LIBRARY_PATH=/opt/lib; exec \"$@\"", "--"]
