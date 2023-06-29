FROM golang:1.20 as builder
LABEL maintainer="Patrick Hermann patrick.hermann@sva.de"

ARG VERSION=""
ARG BUILD_DATE=""
ARG COMMIT=""

WORKDIR /src/
COPY . .

RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /bin/resource-creator \
    -ldflags="-X main.version=${VERSION} -X main.date=${BUILD_DATE} -X main.commit=${COMMIT}"

FROM eu.gcr.io/stuttgart-things/sthings-alpine@sha256:8d67f8b99f4bd4329cbdf5be80f8c8683a8a5cbe341c0860412c984b0a20a621
COPY --from=builder /bin/resource-creator /bin/resource-creator

ENTRYPOINT ["resource-creator"]
