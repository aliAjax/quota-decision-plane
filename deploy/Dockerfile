FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /quota-server ./cmd/server
FROM alpine:3.20
RUN adduser -D -u 10001 quota
USER quota
WORKDIR /app
COPY --from=build /quota-server /app/quota-server
COPY configs/local.yaml /app/configs/local.yaml
EXPOSE 18333
ENTRYPOINT ["/app/quota-server", "-config", "/app/configs/local.yaml"]

