# Сборка веб-клиента и образ Caddy с готовой раздачей SPA.
#
# Контекст — корень репозитория: общий package-lock.json и workspaces api/web.
# Клиенту нужны сгенерированные контракты api/src/gen и их зависимости.

FROM node:24-alpine AS web

WORKDIR /src

COPY package.json package-lock.json ./
COPY api/package.json ./api/package.json
COPY web/package.json ./web/package.json
RUN npm ci

COPY api/src/gen ./api/src/gen
COPY web ./web

RUN npm run build --workspace web


FROM caddy:2.10-alpine

COPY deploy/Caddyfile /etc/caddy/Caddyfile
COPY --from=web /src/web/dist /srv
