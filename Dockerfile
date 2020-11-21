FROM golang:alpine as builder
WORKDIR /go/src
COPY . /go/src/
RUN go build ./cmd/mutator

FROM alpine
COPY --from=builder /go/src/mutator /mutator
ENTRYPOINT /mutator
