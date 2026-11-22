# Проверочный образ: тестовый upstream и клиент маршрутной матрицы.
FROM golang:1.27-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mock .

FROM alpine:3.22
COPY --from=builder /out/mock /usr/local/bin/mock
ENTRYPOINT ["/usr/local/bin/mock"]
