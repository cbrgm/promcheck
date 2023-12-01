# Builder
FROM golang:1.21.4-alpine3.17 AS build

WORKDIR /promcheck

COPY . ./
RUN apk --no-cache add make git gcc libc-dev curl ca-certificates && make release

# Image
FROM alpine:latest

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /promcheck/bin/promcheck_linux_amd64 /bin/promcheck

ENTRYPOINT [ "/bin/promcheck" ]
