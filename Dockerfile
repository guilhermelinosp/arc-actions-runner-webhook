FROM --platform=linux/amd64 alpine:3.21
RUN apk add --no-cache ca-certificates kubectl
COPY runner-webhook /usr/local/bin/runner-webhook
EXPOSE 8080
USER nobody
ENTRYPOINT ["runner-webhook"]
