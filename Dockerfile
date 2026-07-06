FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /runner-webhook .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates kubectl
COPY --from=builder /runner-webhook /usr/local/bin/runner-webhook
EXPOSE 8080
USER nobody
ENTRYPOINT ["runner-webhook"]
