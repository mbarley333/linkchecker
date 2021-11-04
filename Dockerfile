FROM golang:1.17-alpine AS build

ADD . /go/src/linkchecker
WORKDIR /go/src/linkchecker/container/cmd
RUN go mod init linkchecker
RUN CGO_ENABLED=0 go test
RUN CGO_ENABLED=0 go build -o /bin/linkchecker
RUN apk --no-cache add ca-certificates


FROM scratch
COPY --from=build /bin/linkchecker /bin/linkchecker
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/bin/linkchecker"]