FROM ubuntu:24.04

RUN apt-get update && \
    apt-get install -y tcpdump ca-certificates && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir -p /captures

WORKDIR /app
COPY packet-capture /app/packet-capture

ENTRYPOINT ["/app/packet-capture"]
