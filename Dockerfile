FROM registry.access.redhat.com/ubi10/go-toolset:1.26 AS builder

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -o dra-cache-partition ./cmd/driver

FROM registry.access.redhat.com/ubi10/ubi-micro:latest
COPY --from=builder /opt/app-root/src/dra-cache-partition /usr/bin/dra-cache-partition
ENTRYPOINT ["/usr/bin/dra-cache-partition"]
