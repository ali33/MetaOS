FROM golang:1.26-bookworm
RUN apt-get update \
 && apt-get install -y --no-install-recommends libpam0g-dev \
 && rm -rf /var/lib/apt/lists/*
