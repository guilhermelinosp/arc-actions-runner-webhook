FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -ldflags="-s -w" -o /runner-webhook .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /runner-webhook /runner-webhook
EXPOSE 8080
USER 65532
ENTRYPOINT ["/runner-webhook"]
