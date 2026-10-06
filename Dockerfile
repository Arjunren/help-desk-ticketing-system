FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /seed ./cmd/seed

FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app
COPY --from=build /api /usr/local/bin/helpdesk-api
COPY --from=build /migrate /usr/local/bin/helpdesk-migrate
COPY --from=build /seed /usr/local/bin/helpdesk-seed
USER app
EXPOSE 8080
ENTRYPOINT ["helpdesk-api"]
