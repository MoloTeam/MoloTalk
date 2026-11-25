# Сборка веб-клиента и образ Caddy с готовой раздачей SPA.
#
# Контекст — корень репозитория: клиенту нужны сгенерированные контракты
# api/src/gen и их зависимости.

FROM node:24-alpine AS web

WORKDIR /src

COPY web/package.json web/package-lock.json ./web/
RUN npm ci --prefix web

# Сгенерированные дескрипторы импортируют @bufbuild/protobuf и Connect,
# поэтому зависимости api ставятся рядом с исходниками.
COPY api/package.json api/package-lock.json ./api/
RUN npm ci --prefix api --omit=dev

COPY api/src/gen ./api/src/gen
COPY web ./web

RUN npm --prefix web run build


FROM caddy:2.10-alpine

COPY deploy/Caddyfile /etc/caddy/Caddyfile
COPY --from=web /src/web/dist /srv
