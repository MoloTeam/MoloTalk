#!/bin/sh
# Однократная подготовка закрытого bucket. Скрипт идемпотентен: существующий
# bucket сохраняется вместе с объектами, анонимный доступ закрывается повторно.
set -eu

: "${MINIO_ENDPOINT:?MINIO_ENDPOINT is required}"
: "${MINIO_ROOT_USER:?MINIO_ROOT_USER is required}"
: "${MINIO_ROOT_PASSWORD:?MINIO_ROOT_PASSWORD is required}"
: "${MINIO_BUCKET:?MINIO_BUCKET is required}"

echo "preparing bucket ${MINIO_BUCKET}"

mc alias set local "${MINIO_ENDPOINT}" "${MINIO_ROOT_USER}" "${MINIO_ROOT_PASSWORD}" >/dev/null
mc mb --ignore-existing "local/${MINIO_BUCKET}"
mc anonymous set none "local/${MINIO_BUCKET}"

echo "bucket ${MINIO_BUCKET} is ready"
