FROM golang:1.20 as builder
LABEL maintainer="Patrick Hermann patrick.hermann@sva.de"

ARG VERSION=""
ARG BUILD_DATE=""
ARG COMMIT=""
ARG GIT_PAT=""

WORKDIR /src/
COPY . .

RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /bin/mso-informer \
    -ldflags="-X main.version=v${VERSION} -X main.date=${BUILD_DATE} -X main.commit=${COMMIT}"

FROM alpine:3.16.0
COPY --from=builder /bin/mso-informer /bin/mso-informer

ENTRYPOINT ["mso-informer"]
