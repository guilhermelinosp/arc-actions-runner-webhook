FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -ldflags="-s -w" -o /runner-webhook .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates kubectl
COPY --from=builder /runner-webhook /usr/local/bin/runner-webhook
EXPOSE 8080
USER nobody
ENTRYPOINT ["runner-webhook"]
