FROM golang:1.25 AS builder

COPY . /src
WORKDIR /src

# The generated stubs are committed, so the image build only needs to compile
# the service itself.
RUN GOPROXY=https://goproxy.cn make build

FROM debian:stable-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
		ca-certificates \
		netbase \
		tzdata \
		&& rm -rf /var/lib/apt/lists/ \
		&& apt-get autoremove -y && apt-get autoclean -y

COPY --from=builder /src/bin /app
COPY --from=builder /src/configs /app/configs
COPY --from=builder /src/docs /app/docs
COPY --from=builder /src/web /app/web

WORKDIR /app

# HTTP, gRPC, and the SQLite file plus the generated password key.
EXPOSE 8000
EXPOSE 9000
VOLUME ["/app/data"]

# Point data.database.source at /app/data/nagisa.db so the database and the
# password key survive a container replacement.
CMD ["./nagisa", "-conf", "/app/configs"]
