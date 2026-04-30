FROM debian:bookworm-slim
RUN set -Eeuo pipefail; \
	apt-get update; \
	apt-get install -y --no-install-recommends curl; \
	rm -rf /var/lib/apt/lists/*
