FROM ubuntu:22.04
ARG TARGETOS
ARG TARGETARCH
ARG BUILDOS

EXPOSE 3333

RUN echo ${TARGETARCH}
COPY ./_build/${TARGETOS}_${TARGETARCH}/bin/dqlite-bench /dqlite-bench

ENTRYPOINT ["/dqlite-bench"]