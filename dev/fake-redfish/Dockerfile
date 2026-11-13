FROM --platform=$BUILDPLATFORM golang:1.23 AS builder

WORKDIR /workspace
COPY main.go .

ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -o fake-redfish main.go

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /workspace/fake-redfish /fake-redfish
ENTRYPOINT ["/fake-redfish"]
