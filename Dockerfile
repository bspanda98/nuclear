# Build nuclear in a stock Go builder container
FROM golang:1.13.7-alpine as builder

RUN apk add --no-cache make gcc musl-dev linux-headers

ADD . /nuclear
RUN cd /nuclear && make geth

# Pull nuclear into a second stage deploy alpine container
FROM alpine:latest

RUN apk add --no-cache ca-certificates
COPY --from=builder /nuclear/build/bin/nuclear /usr/local/bin/

EXPOSE 39796 39795 39797 39797/udp
ENTRYPOINT ["nuclear"]
