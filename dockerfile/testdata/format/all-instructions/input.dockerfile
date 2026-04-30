# syntax=docker/dockerfile:1
FROM golang:1.21 AS builder
ARG VERSION=dev
LABEL maintainer="test@example.com" version="1.0"
ENV GOPATH /go
WORKDIR /src
COPY --from=builder --chown=user:group /src /dst
COPY ["src1", "src2", "/dst/"]
ADD https://example.com/file.tar.gz /tmp/
RUN set -eux; \
	apt-get update; \
	apt-get install -y --no-install-recommends curl; \
	rm -rf /var/lib/apt/lists/*
EXPOSE 8080 8443
USER nobody:nogroup
VOLUME /var/log
VOLUME ["/var/data", "/var/cache"]
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=5s CMD curl -f http://localhost/
HEALTHCHECK NONE
SHELL ["/bin/sh", "-c"]
ONBUILD RUN echo "triggered"
CMD ["server", "--port", "8080"]
ENTRYPOINT ["/usr/local/bin/entrypoint"]
