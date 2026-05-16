###-------------------------lodev-utilities-base--------------------------
FROM alpine:3.23 AS lodev-utilities-base
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ENV CAROOT="/mnt/lodev_default/traefik/mkcert"
RUN apk add --no-cache bash curl jq perl-utils wget yq nss wget openssl

RUN curl --fail -JL -s -o /usr/local/bin/mkcert "https://dl.filippo.io/mkcert/latest?for=linux/${TARGETPLATFORM#linux/}" && chmod +x /usr/local/bin/mkcert

RUN mkdir -p "$CAROOT" && mkcert -install
### -------------------------END lodev-utilities-base--------------------------

### -----------------------------lodev-utilities-------------------------------
FROM lodev-utilities-base AS lodev-utilities
COPY --from=lodev-utilities-base / /

CMD ["bash"]
### -------------------------END lodev-utilities-------------------------------
