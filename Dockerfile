FROM golang:alpine AS builder

COPY . /go/src/github.com/GovAuCSU/ctlog-acquisition

RUN apk add --no-cache git

# If we don't disable CGO, the binary won't work in the scratch image. Unsure why?
RUN cd /go/src/github.com/GovAuCSU/ctlog-acquisition && CGO_ENABLED=0 GO111MODULE=on go build github.com/GovAuCSU/ctlog-acquisition
RUN cd /go/src/github.com/GovAuCSU/ctlog-acquisition/cmd && CGO_ENABLED=0 GO111MODULE=on go build github.com/GovAuCSU/ctlog-acquisition/cmd

FROM scratch

COPY --from=builder /go/src/github.com/GovAuCSU/ctlog-acquisition/cmd/cmd /go/bin/ctlog-acquisition
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENTRYPOINT ["/go/bin/ctlog-acquisition"]
CMD []
