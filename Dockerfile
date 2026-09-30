# Base image for CI agents. Releases are installed on demand into /mvm;
# mount a volume there to keep them between builds.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates git python3 make g++ \
 && rm -rf /var/lib/apt/lists/* \
 && mkdir -p /mvm && chmod 1777 /mvm
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/mvm /usr/local/bin/mvm
ENV MVM_HOME=/mvm PATH=/mvm/bin:$PATH
RUN mvm init >/dev/null
