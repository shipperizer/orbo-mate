FROM golang:latest AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o orbo-mate main.go

FROM gcr.io/distroless/static-debian12:latest
COPY --from=builder /app/orbo-mate /orbo-mate
ENTRYPOINT ["/orbo-mate"]
