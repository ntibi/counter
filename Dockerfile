FROM golang:alpine AS builder

WORKDIR /app
COPY go.mod go.sum* ./
COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server
RUN CGO_ENABLED=0 go build -o import ./cmd/import

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/server /usr/local/bin/server
COPY --from=builder /app/import /usr/local/bin/import
EXPOSE 8080
CMD ["server"]
