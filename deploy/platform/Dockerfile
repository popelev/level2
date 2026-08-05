FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o /out/collector ./cmd/collector

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/collector /collector
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/collector"]
