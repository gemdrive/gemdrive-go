# We need to use a builder because we don't have access to the mkdir command
# from the scratch image
FROM alpine:3.16 as builder

RUN ["mkdir", "/data"]


FROM scratch
#FROM alpine:3.16

COPY --from=builder /data /

COPY gemdrive-server /

ENTRYPOINT ["/gemdrive-server"]

CMD ["-dir", "/data"]
