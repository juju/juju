# Build platform-specific wheels. All arches use this stage so the
# final install is offline and consistent; on s390x this also compiles
# cryptography from source (no PyPI wheel exists). The toolchain is
# discarded with this stage.
FROM public.ecr.aws/ubuntu/ubuntu:24.04 AS wheelhouse
ARG TARGETARCH
RUN apt-get update \
    && apt-get install -y --no-install-recommends python3-pip \
    && if [ "$TARGETARCH" = "s390x" ]; then \
        apt-get install -y --no-install-recommends \
        build-essential libssl-dev libffi-dev python3-dev pkg-config; \
    fi \
    && rm -rf /var/lib/apt/lists/*
COPY docker-staging/requirements.txt /tmp/requirements.txt
RUN pip3 wheel --wheel-dir /wheels -r /tmp/requirements.txt

FROM public.ecr.aws/ubuntu/ubuntu:24.04
ARG TARGETOS
ARG TARGETARCH

# Add the syslog user for audit logging.
RUN useradd --system --no-create-home --shell /usr/sbin/nologin syslog
# Add the juju and sjuju user for rootless agents.
# 170 and 171 uid/gid must be updated here and in caas/kubernetes/provider/constants/constants.go
RUN groupadd --gid 170 juju
RUN useradd --uid 170 --gid 170 --no-create-home --shell /usr/bin/bash juju
RUN groupadd --gid 171 sjuju
RUN useradd --uid 171 --gid 171 --no-create-home --shell /usr/bin/bash sjuju
RUN mkdir -p /etc/sudoers.d && echo "sjuju ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/sjuju

# Some Python dependencies.
RUN DEBIAN_FRONTEND=noninteractive apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    sudo \
    python3-yaml \
    python3-pip \
    python3-setuptools \
    # for debug-hooks.
    tmux byobu \
    curl iproute2 \
    && rm -rf /var/lib/apt/lists/* \
    && rm -rf /root/.cache

# Install the standard charm dependencies.
ENV WHEELHOUSE=/tmp/wheelhouse
ENV PIP_WHEEL_DIR=/tmp/wheelhouse
ENV PIP_FIND_LINKS=/tmp/wheelhouse

COPY --from=wheelhouse /wheels /tmp/wheelhouse
COPY docker-staging/requirements.txt /tmp/requirements.txt
RUN pip3 install --no-index --find-links=/tmp/wheelhouse --break-system-packages \
    -r /tmp/requirements.txt

WORKDIR /var/lib/juju
COPY ${TARGETOS}_${TARGETARCH}/bin/jujud /opt/
COPY ${TARGETOS}_${TARGETARCH}/bin/jujuc /opt/
COPY ${TARGETOS}_${TARGETARCH}/bin/containeragent /opt/
COPY ${TARGETOS}_${TARGETARCH}/bin/pebble /opt/

ENTRYPOINT ["sh", "-c"]
