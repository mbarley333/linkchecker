FROM golang:1.17-alpine AS build

ADD . /go/src/linkchecker
WORKDIR /go/src/linkchecker/cmd
RUN CGO_ENABLED=0 go test
RUN CGO_ENABLED=0 go build -o /bin/linkchecker

FROM scratch
COPY --from=build /bin/linkchecker /bin/linkchecker
ENTRYPOINT ["/bin/linkchecker"]