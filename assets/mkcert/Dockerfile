FROM alpine:3.23
ARG TARGETARCH
# Pinned and checked: this binary makes the CA your browser trusts.
RUN apk add --no-cache ca-certificates curl \
    && case "$TARGETARCH" in \
      amd64) sum=6d31c65b03972c6dc4a14ab429f2928300518b26503f58723e532d1b0a3bbb52 ;; \
      arm64) sum=b98f2cc69fd9147fe4d405d859c57504571adec0d3611c3eefd04107c7ac00d0 ;; \
      *) echo "no mkcert for $TARGETARCH" >&2; exit 1 ;; esac \
    && curl -fsSL -o /usr/local/bin/mkcert "https://github.com/FiloSottile/mkcert/releases/download/v1.4.4/mkcert-v1.4.4-linux-${TARGETARCH}" \
    && echo "$sum  /usr/local/bin/mkcert" | sha256sum -c - \
    && chmod +x /usr/local/bin/mkcert
ENTRYPOINT ["mkcert"]
