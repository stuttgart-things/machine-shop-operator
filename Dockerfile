# Build the manager binary
FROM golang:1.19 as builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager main.go

FROM debian AS cert-env

# Add CA files
ARG VAULT_URL_LABUL=https://vault.labul.sva.de:8200
ARG VAULT_URL_LABDA=https://vault.tiab.labda.sva.de:8200
ARG VAULT_URL_LABUL_VSPHERE=https://vault-vsphere.labul.sva.de:8200
ARG VAULT_URL_LABUL_PVE=https://vault-pve.labul.sva.de:8200
ARG VAULT_URL_LABDA_VSPHERE=https://vault-vsphere.tiab.labda.sva.de:8200

RUN apt update -qqq && apt install -y wget
RUN wget -O /usr/local/share/ca-certificates/labul-ca.crt ${VAULT_URL_LABUL}/v1/pki/ca/pem --no-check-certificate \
    && wget -O /usr/local/share/ca-certificates/labda-ca.crt ${VAULT_URL_LABDA}/v1/pki/ca/pem --no-check-certificate \
    && wget -O /usr/local/share/ca-certificates/scc-ca.crt ${VAULT_URL_SSC}/v1/pki/ca/pem --no-check-certificate \
    && wget -O /usr/local/share/ca-certificates/labul-vsphere-ca.crt ${VAULT_URL_LABUL_VSPHERE}/v1/pki/ca/pem --no-check-certificate \
    && wget -O /usr/local/share/ca-certificates/labul-pve.crt ${VAULT_URL_LABUL_PVE}/v1/pki/ca/pem --no-check-certificate \
    && wget -O /usr/local/share/ca-certificates/labul-pve.crt ${VAULT_URL_LABDA_VSPHERE}/v1/pki/ca/pem --no-check-certificate

RUN apt update -qqq && \
    apt install -yqqq ca-certificates openssh-client sshpass && \
    update-ca-certificates

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=cert-env /etc/ssl/certs /etc/ssl/certs
USER 65532:65532

ENTRYPOINT ["/manager"]
