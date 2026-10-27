FROM golang:1.26.5-alpine3.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /worker .

FROM alpine:3.23.3
RUN apk add --no-cache ca-certificates && addgroup -S worker && adduser -S -G worker worker
USER worker
COPY --from=build /worker /worker
EXPOSE 8080
ENTRYPOINT ["/worker"]
