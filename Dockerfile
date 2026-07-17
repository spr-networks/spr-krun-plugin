# syntax=docker/dockerfile:1@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89
ARG UBUNTU_REF=ubuntu:24.04@sha256:4fbb8e6a8395de5a7550b33509421a2bafbc0aab6c06ba2cef9ebffbc7092d90
ARG CONTAINER_TEMPLATE_REF=ghcr.io/spr-networks/container_template@sha256:869ada7b121e9a0c552674042d32e801da3c4d04145638d9e722918c6377e65f
ARG UBUNTU_SNAPSHOT=20260601T000000Z

FROM ${CONTAINER_TEMPLATE_REF} AS base

FROM ${UBUNTU_REF} AS builder
ARG UBUNTU_SNAPSHOT
ARG GO_VERSION=1.25.12
ARG GO_SHA256_AMD64=234828b7a89e0e303d2556310ee549fbcf253d28de937bac3da13d6294262ac1
ARG GO_SHA256_ARM64=8b5884aef89600aef5b0b051fb971f11f49bb996521e911f30f02a66884f7bd2
ARG TARGETARCH
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN set -eux; \
    printf 'Types: deb\nURIs: https://snapshot.ubuntu.com/ubuntu/%s\nSuites: noble noble-updates noble-security\nComponents: main restricted universe multiverse\nSigned-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg\n' "${UBUNTU_SNAPSHOT}" > /etc/apt/sources.list.d/ubuntu.sources; \
    printf 'APT::Install-Recommends "false";\nAcquire::Check-Valid-Until "false";\n' > /etc/apt/apt.conf.d/99reproducible; \
    apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates wget \
    && rm -rf /var/lib/apt/lists/* /var/log/* /var/cache/ldconfig/aux-cache
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) GO_SHA256="${GO_SHA256_AMD64}";; \
      arm64) GO_SHA256="${GO_SHA256_ARM64}";; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1;; \
    esac; \
    wget -q "https://dl.google.com/go/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz"; \
    echo "${GO_SHA256}  go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" | sha256sum -c -; \
    tar -C /usr/local -xzf "go${GO_VERSION}.linux-${TARGETARCH}.tar.gz"
ENV PATH=/usr/local/go/bin:${PATH} GOTOOLCHAIN=local CGO_ENABLED=0
WORKDIR /src
COPY code/ ./
RUN --mount=type=tmpfs,target=/root/go \
    go test ./... && \
    go build -trimpath -ldflags "-s -w" -o /spr-krun-vsock-proxy .

FROM base
LABEL org.opencontainers.image.source="https://github.com/spr-networks/spr-krun-plugin"
COPY --from=builder /spr-krun-vsock-proxy /usr/local/bin/
COPY scripts/spr-krun-init /usr/local/bin/
RUN chmod 0755 \
      /usr/local/bin/spr-krun-init \
      /usr/local/bin/spr-krun-vsock-proxy

ENTRYPOINT ["/usr/local/bin/spr-krun-init"]
