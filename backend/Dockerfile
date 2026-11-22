# Сборка пяти процессов из одного Go-модуля. Версия builder подобрана под
# backend/go.mod (go 1.27) и фиксируется тегом образа.
FROM golang:1.27-alpine AS builder

WORKDIR /src

# Сначала манифесты модуля: слой кешируется, пока не изменились зависимости.
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

# Общая основа итоговых образов: непривилегированный пользователь и средство
# HTTP-проверки, которое использует healthcheck Compose.
FROM alpine:3.22 AS runtime
RUN command -v wget >/dev/null
RUN adduser -D -u 10001 -h /app app
WORKDIR /app
USER app

FROM runtime AS auth
COPY --from=builder /out/auth /app/bin/auth
EXPOSE 8081
ENTRYPOINT ["/app/bin/auth"]

FROM runtime AS chat
COPY --from=builder /out/chat /app/bin/chat
EXPOSE 8082
ENTRYPOINT ["/app/bin/chat"]

FROM runtime AS media
COPY --from=builder /out/media /app/bin/media
EXPOSE 8083
ENTRYPOINT ["/app/bin/media"]

FROM runtime AS presence
COPY --from=builder /out/presence /app/bin/presence
EXPOSE 8084
ENTRYPOINT ["/app/bin/presence"]

FROM runtime AS gateway
COPY --from=builder /out/gateway /app/bin/gateway
EXPOSE 8085
ENTRYPOINT ["/app/bin/gateway"]
