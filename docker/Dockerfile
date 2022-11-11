# It would be nice to use a scratch image, but currently don't want to deal
# with the complexities of things like TLS certs.

FROM alpine:3.16 as builder

RUN ["mkdir", "/drive"]


#FROM scratch
#FROM alpine:3.16
#COPY --from=builder /data /

COPY gemdrive-server /

ENTRYPOINT ["/gemdrive-server"]

CMD ["-dir", "/drive"]
