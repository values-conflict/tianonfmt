FROM debian:bookworm-slim

RUN set -eux; \
	apt-get update; \
	apt-get install -y --no-install-recommends \
		curl \
		git \
	; \
	curl -fsSL https://example.com/install.sh | sh; \
	rm -rf /var/lib/apt/lists/*
