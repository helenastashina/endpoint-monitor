FROM golang:1.27-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/endpoint-monitor ./cmd/app

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /bin/endpoint-monitor /app/endpoint-monitor

EXPOSE 8080

CMD ["/app/endpoint-monitor"]
