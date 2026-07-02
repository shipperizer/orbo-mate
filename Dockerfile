FROM --platform=$BUILDPLATFORM golang:latest AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o orbo-mate main.go

FROM gcr.io/distroless/static-debian12:latest
LABEL org.opencontainers.image.source="https://github.com/shipperizer/orbo-mate"
COPY --from=builder /app/orbo-mate /orbo-mate
ENTRYPOINT ["/orbo-mate"]
