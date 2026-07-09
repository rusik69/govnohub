FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG SERVICE=api-server
RUN CGO_ENABLED=0 go build -o /out/service ./cmd/${SERVICE}

FROM alpine:3.24
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY --from=builder /out/service /app/service
USER 1000
ENTRYPOINT ["/app/service"]
