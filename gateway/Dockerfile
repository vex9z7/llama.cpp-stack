FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY gateway ./gateway
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/llama-gateway ./gateway/cmd/gateway

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/llama-gateway /usr/local/bin/llama-gateway
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/llama-gateway"]
