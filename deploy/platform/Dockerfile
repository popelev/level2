FROM node:22-bookworm AS ui
WORKDIR /web
COPY web/package.json ./
RUN npm install
COPY web/ ./
RUN npm run build

FROM golang:1.24-bookworm AS build
WORKDIR /src
ENV GOTOOLCHAIN=auto
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/collector ./cmd/collector

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/collector /collector
COPY --from=ui /web/dist /usr/share/level2/ui
EXPOSE 8080
ENTRYPOINT ["/collector"]
