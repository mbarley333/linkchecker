FROM alpine AS build
RUN apk --no-cache add ca-certificates

FROM scratch
COPY linkchecker /bin/linkchecker
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/bin/linkchecker"]