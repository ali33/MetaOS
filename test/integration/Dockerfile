FROM golang:1.26-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends libpam0g-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir -p web/dist && { [ -e web/dist/index.html ] || echo '<!doctype html><title>MetaOS</title>' > web/dist/index.html; }
RUN CGO_ENABLED=1 go build -trimpath -o /out/ ./cmd/...

FROM debian:bookworm
ENV container=docker
RUN apt-get update && apt-get install -y --no-install-recommends \
      systemd systemd-sysv dbus libpam0g libpam-modules openssl sudo procps ca-certificates \
 && rm -rf /var/lib/apt/lists/*
RUN useradd --system --home-dir /var/lib/metaos --shell /usr/sbin/nologin metaos \
 && useradd -m -s /bin/bash alice && echo 'alice:alice-pass-1' | chpasswd \
 && useradd -m -s /bin/bash -G sudo bob && echo 'bob:bob-pass-1' | chpasswd
COPY --from=build /out/metaos-ws /out/metaos-bridge /usr/lib/metaos/
COPY --from=build /out/metaos-auth /usr/lib/metaos/metaos-auth
RUN chown root:metaos /usr/lib/metaos/metaos-auth && chmod 4750 /usr/lib/metaos/metaos-auth
COPY --chmod=0644 packaging/pam.d/metaos /etc/pam.d/metaos
COPY --chmod=0644 packaging/metaos.service /etc/systemd/system/metaos.service
RUN mkdir -p /etc/metaos/tls \
 && openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 3650 \
      -subj /CN=metaos-it -keyout /etc/metaos/tls/key.pem -out /etc/metaos/tls/cert.pem \
 && chown -R root:metaos /etc/metaos/tls && chmod 0640 /etc/metaos/tls/key.pem \
 && systemctl enable metaos.service
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
