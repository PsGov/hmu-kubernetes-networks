FROM golang:1.26.10-alpine3.24 AS build
WORKDIR /src
COPY go.mod .
COPY *.go .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /api .

FROM alpine:3.24.3
RUN addgroup -S app && adduser -S -G app app
USER app
COPY --from=build /api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
