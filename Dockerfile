FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /u ./cmd/u

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /u /app/u

VOLUME ["/data"]

ENV U_DB_PATH=/data/u.db

EXPOSE 8080

ENTRYPOINT ["/app/u", "/data/config.yaml"]
