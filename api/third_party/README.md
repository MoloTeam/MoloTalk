# Внешние proto-зависимости

Каталог содержит внешние `.proto`, необходимые контрактам MoloTalk. Файлы
закоммичены, чтобы сборка, линт и генерация не зависели от сети и реестра Buf.
Каждый файл скачан из закреплённого источника, лицензии лежат в `licenses/`.

| Файл | Источник | Закрепление | SHA-256 | Лицензия |
|---|---|---|---|---|
| `proto/buf/validate/validate.proto` | `bufbuild/protovalidate` | тег `v1.2.2` | `a1dc1163ebac930b3b070655b91f2bc78df8b4aea940021661fe5ec7904a7b88` | Apache-2.0 |
| `proto/google/api/field_behavior.proto` | `googleapis/googleapis` | коммит `0616ac54ade8ad70502627e445fa786f93154459` | `e2a1ac88c49909d4579bacbb18519085e82da32c2c7bb15c31037f3be19f667f` | Apache-2.0 |

Импорты `google/protobuf/*.proto` (descriptor, duration, field_mask, timestamp)
предоставляются компилятором Buf и в каталог не копируются.

## Обновление

1. Скачать файл из указанного источника на новом закреплённом теге или
   коммите.
2. Пересчитать SHA-256 и обновить строку в таблице.
3. При изменении версии protovalidate обновить `buf.build/go/protovalidate` в
   `backend/go.mod` и правила в proto, затем перегенерировать артефакты и golden.
4. Проверить `buf lint`, `buf build` и `npm --prefix api run check`.
