FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY hello /usr/local/bin/hello
USER nobody
CMD ["hello"]
