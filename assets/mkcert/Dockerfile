FROM alpine:3.20
ARG TARGETARCH
RUN apk add --no-cache ca-certificates curl \
    && curl -fsSL -o /usr/local/bin/mkcert "https://dl.filippo.io/mkcert/latest?for=linux/${TARGETARCH}" \
    && chmod +x /usr/local/bin/mkcert
ENTRYPOINT ["mkcert"]
