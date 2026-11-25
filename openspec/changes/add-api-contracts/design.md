# Design

## Context

Цель приведена в proposal. В рабочей копии существуют Go-каркас пяти процессов и эксплуатационные HTTP-маршруты. RPC и WebSocket не реализованы. Caddy уже разрешает 29 внешних методов. Ветка предложения и последующей реализации — `feat/6-api-contracts`.

Прочитаны актуальные материалы отчёта и схема #5. SHA-256 фиксируют исследованный снимок, а не объявляют прежнее значение 4000 действующим требованием.

| Источник | SHA-256 |
|---|---|
| report_HLD.docx | `cfa97ee9a2dc0605bbfc9da11019841eab2d31934b1663a85c285d989a4797d6` |
| materials_updated/openapi.json | `17eedbb8627022554792728a4bb36afea4b8b535932ef6ef1a3c5b93d2a1bfeb` |
| materials_updated/gateway.proto | `f90e96aed40c146ddb614e7c61dbf1982543d22fd9a07ee8f46ca5a7595fe4e8` |
| docs/hld/schema.sql в рабочей копии #5 | `c9b12e811e35202cbebaa0c25d6a44b1f0766f4369cb3d6d2ba1e36d292a55d1` |

Текущий снимок schema.sql ещё содержит 4000, однако решение владельца для #5 и #6 — 4096. Исправление файлов #5 в этот change не входит. Форматы identifier, display_name и названия группы сверены с SQL, revision хранится как PostgreSQL integer, size_bytes — bigint с ограничениями 5/20 МиБ.

## Goals / Non-Goals

**Goals** — единые proto-типы и правила, готовые генераторы и валидаторы, проверяемые JSON/бинарные представления, независимая работа над RPC и клиентом.

**Non-Goals** — бизнес-обработчики, сетевой Gateway, миграции, fixtures, изменение HLD, Caddy, CI и Compose приложения. Транспортные типы не заменяют доменные модели. Описание инструментов ниже является планом реализации; в репозитории на этом этапе меняются только артефакты change.

## Decisions

### D1. Единственный источник и структура

Proto определяет сервисы, DTO и ограничения protovalidate. OpenAPI, Go и TypeScript — производные файлы. Отдельный слой сопоставления схем и собственная проверка их эквивалентности не создаются.

```text
api/
  proto/molotalk/v1/                 # публичные RPC, общие типы, Gateway среза C
  proto/molotalk/internal/v1/        # внутренние RPC, импорт публичных DTO допустим
  buf.yaml
  third_party/proto/                 # закоммиченные buf.validate и google.api
  buf.gen.go.yaml
  buf.gen.ts.yaml
  buf.gen.openapi.public.yaml
  openapi/base.public.yaml           # Bearer и исключения для 3 методов без access token
  openapi/base.internal.yaml         # служебный Bearer
  gen/openapi/public.json            # одновременно проверяемый golden
  gen/openapi/internal.json          # одновременно проверяемый golden
  src/gen/                          # только публичные TS-дескрипторы и их зависимости
  package.json
  package-lock.json
  README.md
backend/
  go.mod                            # tool + закреплённые версии
  go.sum
  gen/molotalk/v1/
  gen/molotalk/internal/v1/
  internal/platform/rpc/            # минимальные общие настройки и отображение ошибок
```

Base-файлы не повторяют DTO и их ограничения. Они содержат только HTTP-метаинформацию, которую proto/валидатор автоматически не задаёт. Публикация в npm/BSR не требуется.

### D2. Пакеты и видимость

| Пакет | Сервис | Методы |
|---|---|---|
| molotalk.v1 | AuthService | Register, Login, RefreshToken, Logout, GetMe, UpdateProfile, ChangePassword, SearchUsers |
| molotalk.v1 | ChatService | ListChats, GetChat, CreateDirectChat, CreateGroupChat, AddGroupMember, RemoveGroupMember, LeaveGroup, SetGroupMemberRole, SendMessage, EditMessage, DeleteMessage, GetMessages, ConfirmDelivery, MarkRead, GetMessageReceipts, GetReadStates |
| molotalk.v1 | MediaService | PrepareUpload, CompleteUpload, GetAttachment, GetDownloadURL |
| molotalk.v1 | PresenceService | GetPresence |
| molotalk.internal.v1 | AuthInternalService | IntrospectSession |
| molotalk.internal.v1 | ChatInternalService | CheckChatAccess, GetUserChatIds |
| molotalk.internal.v1 | PresenceInternalService | Heartbeat, Disconnect |

Разделение обеспечивается каталогами и пакетами. Go получает оба пакета. Вход генерации TypeScript ограничивается публичным каталогом; публичные файлы не импортируют внутренние. Зависимости WKT и buf.validate не являются внутренними RPC. OpenAPI генерируется отдельно для пакетов штатным фильтром `services` и `strategy: all`. Ручной список разрешённых имён для этой генерации не нужен.

Все 29 внешних путей остаются `/molotalk.v1.<Service>/<Method>`. Например, внутренний путь становится `/molotalk.internal.v1.AuthInternalService/IntrospectSession`. Caddy эти пути по-прежнему не пропускает. Будущие AuthClient, ChatAccessClient и вызовы Presence должны использовать новые сгенерированные клиенты. Само размещение во внутреннем пакете не заменяет проверку служебной авторизации.

### D3. Типы и форматные правила

- Настоящие proto-enum имеют стандартный префикс Buf и значение `*_UNSPECIFIED = 0`. Пример JSON — `CHAT_KIND_GROUP`, а не прежний `GROUP`. Обязательное enum-поле имеет required, `defined_only: true` и `not_in: [0]`. Для optional enum правила применяются при наличии, явный ноль также запрещён, если ноль не имеет продуктового смысла.
- Timestamp — `google.protobuf.Timestamp`, пустые RPC-запросы и ответы — `google.protobuf.Empty`. Timestamp выдаётся UTC RFC3339, стандартный парсер также принимает смещение UTC и нормализует его. Никаких проверок относительно системного времени в форматном валидаторе не требуется.
- Id остаётся строкой с `string.pattern = ^[0-7][0-9A-HJKMNP-TV-Z]{25}$` во всех местах, включая элементы массивов. Для обязательных scalar-полей, где нужно различить отсутствие и ноль/false, используется optional и required. Optional update-поля сохраняют presence.
- identifier — 1–254 Unicode-символа в нижнем регистре. Для запрета прописных и титульных букв проверен RE2-шаблон `^[^\p{Lu}\p{Lt}]*$`; одной `lowerAscii()` для кириллицы недостаточно. Длины и шаблон задаются protovalidate. Нормализация и уникальность остаются ответственностью Auth.
- display_name — 1–64, название группы — 1–100, пароль — 6–64, текст сообщения — до 4096 Unicode-символов. Состав пароля остаётся доменным правилом Auth. Непустота содержательного сообщения, доступ к файлу и лимит состава группы остаются прикладными предусловиями.
- Вложения — до 5, элементы проходят ULID-правило. История и ListChats — limit 1–100 при наличии, отсутствие означает 50. SearchUsers — 1–50, отсутствие означает 20. Значения по умолчанию описаны в proto-комментариях, а не подменяются нулём enum/scalar.

#### Аудит int64 и числовых полей

В исходном OpenAPI найдено 13 числовых полей либо десятичных строк, к которым мог применяться int64. Они перечислены полностью. Отдельно добавлено поле события из §5.8.

| Поля | Целевой тип | Граница / основание |
|---|---|---|
| Chat.unreadCount; ReadState.unreadCount | int32 | 0..2147483647, контрактные счётчики, без молчаливого переполнения при преобразовании SQL COUNT |
| ReceiptSummary.recipientCount, deliveredCount, readCount | int32 | Неотрицательные счётчики, реальная аудитория ограничена составом чата |
| Message.revision; EditMessage.expectedRevision; DeleteMessageResponse.revision; MessageDeleted.revision | int32 | 1..2147483647, revision в #5 имеет тип integer |
| SearchUsers.limit | optional int32 | 1..50 |
| ListChats.limit; GetMessages.limit | optional int32 | 1..100 |
| Attachment.sizeBytes; PrepareUpload.sizeBytes | int32 | 0..20971520, для аватара дополнительно 5242880, обе границы меньше INT32_MAX |

Произвольных прикладных int64 не остаётся, поэтому искусственный пример `9007199254740993` удаляется. Стандартное Timestamp.seconds остаётся int64 внутри WKT, на JSON-проводе передаётся timestamp-строкой; его не заменяют своим типом. Поле sequence старого Gateway отсутствует в утверждённом варианте B. Миграция size_bytes из bigint не требуется, Go-адаптер позднее выполняет проверяемое преобразование. Достижение верхней границы revision требует штатного отказа, а не переполнения.

#### Явные значения и presence

Ответы выдают обязательные false, 0 и пустые массивы. Для Go используется политика `protojson.EmitDefaultValues` с сохранением отсутствующих presence-полей; конкретная настройка кодека Connect подтверждается тестом. Для Protobuf-ES проверен `alwaysEmitImplicit: true`. Нельзя включать общий EmitUnpopulated, превращающий отсутствующие message-поля в null. Optional bool/int с required сохраняют допустимые false/0, когда они разрешены диапазоном.

Для repeated-поля proto3 не различает отсутствие и пустой массив. `buf.validate.field.required` на нём означает непустой массив и не подходит для результата `[]`. Документированное исключение — стандартная proto-аннотация `google.api.field_behavior = REQUIRED` для присутствия пустых массивов в JSON-документации. Кардинальность и элементы остаются в protovalidate. Это не дополнительный источник схемы. Наличие `[]` в фактическом ответе обеспечивают сериализация и контрактный тест, а не самописный валидатор присутствия.

### D4. Результаты исследования OpenAPI

Исследование выполнено вне репозитория в отдельной временной папке. Созданы минимальные proto, вызваны готовые Buf и protoc-gen-connect-openapi, затем сгенерированные JS-сообщения проверены реальным protovalidate. Полный API пока не реализован; результаты относятся к контрольным конструкциям, а не к уже выполненной полной сверке 34 RPC.

Проверенные версии — Buf 1.75.0, `connectrpc.com/protoc-gen-connect-openapi` 0.30.0 (commit `9f17938b1e91f343a9be552af95c7cfad961d05d`, SHA-256 архива сверена), схема protovalidate v1.2.2 (`4baed765070e580226a0a30bf2a357cc027dc1f7`), Protobuf-ES/runtime 2.17.0, protovalidate-ES 1.3.0, Node 24.11.1. Источник стандартной field_behavior закоммичен в `api/third_party` (см. раздел о внешних proto); исследован файл с SHA-256 `e2a1ac88c49909d4579bacbb18519085e82da32c2c7bb15c31037f3be19f667f`.

| Правило / свойство | Фактический результат генерации | Решение |
|---|---|---|
| text max_len 4096 | `maxLength: 4096` | Генерация без дополнений |
| identifier 1–254, display_name 1–64, group name 1–100, password 6–64 | minLength / maxLength сохранены | Генерация без дублирования правил |
| ULID и Unicode lower-case pattern | pattern сохранён, в том числе для элементов массива | Генерация без дополнений |
| Вложения max_items 5 | `maxItems: 5` и items.pattern | Генерация без дополнений |
| limit 1–100; sizeBytes 0–20971520 | minimum / maximum, integer int32 | Генерация без дополнений; другие числовые границы задаются теми же правилами |
| required на optional scalar и message | Поле попадает в родительский required | Сохраняется; отдельно учтён null ниже |
| repeated.required | Поле попадает в required, но runtime отвергает [] | Не использовать для допускающих пустой результат массивов |
| repeated с google.api.field_behavior REQUIRED | Поле присутствует в required, runtime допускает [] | Стандартное документированное исключение D3 |
| Enum defined_only и not_in [0] | Перечень enum и `not.enum: [*_UNSPECIFIED]` | Валидатор отвергает 0 и неизвестные значения |
| Message-level oneof(required) ReceiptUpdated | Сгенерирован машинный `not/anyOf/required`, запрещающий оба и ни одного поля | Использовать готовое правило, CEL здесь не нужен |
| Произвольный CEL | Только текст description, не исполняемый JSON Schema | Protovalidate остаётся исполнителем. Исключение явно документируется, собственный транслятор CEL не создаётся |
| Timestamp / Empty | date-time строка / пустой объект | WKT, без собственных DTO-обёрток |
| bearerAuth/serviceBearer без base | Не появляются автоматически из protovalidate | Security — не правило поля. Использовать штатные base-файлы |
| Base с двумя securitySchemes и переопределением операции | Обе схемы сохранены; Login имеет `security: []`, Check — serviceBearer, остальные наследуют bearerAuth | Штатный base покрывает отсутствие security без обработки готового JSON |
| Optional и null | Генератор допускает null в optional-ветви даже при required; runtime для обязательного поля строже | Документированное ограничение генератора: authoritative validation — protovalidate, ответы null не выдают; схему вручную не переписывать |
| Значения по умолчанию limit 50/20 | Правило диапазона не порождает JSON Schema default | Описать в proto-комментарии; семантика отсутствия проверяется прикладной карточкой |

В 0.30.0 protovalidate включён без отдельной feature: `features=protovalidate` отвергается. Для Connect JSON достаточно `features=connectrpc`, `content-types=json`, `format=json`, `path=...`, `strategy: all`; base задаётся локальному плагину. Это проверено на исполняемом инструменте, а не перенесено из устаревшей документации.

Результаты исполнения примеров

- 4096 символов вне BMP — valid, 4097 — invalid.
- 6 вложений, limit 0/101, enum 0/99 — invalid.
- Нижний регистр кириллицы — valid, прописная кириллица — invalid.
- Required bool со значением false — valid.
- Repeated с protovalidate.required и [] — invalid; repeated с field_behavior REQUIRED и [] — valid.
- ReceiptUpdated со сводкой либо подтверждением — valid; с обоими либо без обоих — invalid.
- Protobuf-ES с alwaysEmitImplicit сохранил пустые массивы, false, enum-префикс и Timestamp-строку; отсутствующие optional-поля не появились.
- `buf breaking` вернул 100 при замене номера поля token с 1 на 2 и 0 после возврата исходного определения.

Источники — [плагин 0.30.0](https://github.com/connectrpc/protoc-gen-connect-openapi/tree/v0.30.0), его `protovalidate.md`, [правила protovalidate v1.2.2](https://github.com/bufbuild/protovalidate/blob/v1.2.2/proto/protovalidate/buf/validate/validate.proto). Текстовые результаты сохранены здесь; исследовательские программы не добавляются в репозиторий.

### D5. ReceiptUpdated и ошибки

ReceiptUpdated имеет обязательные ULID chat_id/message_id и optional receipt_summary/receipt. Используется `(buf.validate.message).oneof = { fields: ["receipt_summary", "receipt"], required: true }`. Это проверяет ровно одно поле, сохраняя требуемые обычные optional-поля. Подменять его структурным protobuf oneof не требуется.

Автору отправляется receipt_summary. Сеансам читателя — только receipt с userId этого читателя. Проверка соответствия получателю принадлежит Gateway и не может быть выполнена контекстно-независимым валидатором DTO. Групповой список подтверждений отсутствует; автор получает персональные отметки через GetMessageReceipts. Два содержательных поля в одном кадре не объединяются.

Ошибки остаются стандартными Connect code/message/details. Для ошибок формата используется готовый `connectrpc.com/validate` v0.9.0 с `NewServerInterceptor` для Connect-Go v2 и штатными protovalidate violations. Новый interceptor и набор validation-detail не проектируются. Непредвиденная ошибка отображается в безопасный internal без исходного текста зависимости. В тестах сохраняются X-Request-Id и исключение секретов. Auth-бизнес-правила и проверка прав не реализуются в этом change. Поддержка Connect v2 подтверждена go.mod релиза и [документацией interceptor](https://pkg.go.dev/connectrpc.com/validate@v0.9.0).

### D6. Инструменты без Docker и проверка совместимости

В backend/go.mod используются tool-директивы для Buf и трёх Go-плагинов. Начальная закреплённая связка

| Инструмент / runtime | Версия | Лицензия |
|---|---|---|
| github.com/bufbuild/buf/cmd/buf | v1.75.0 | Apache-2.0 |
| google.golang.org/protobuf/cmd/protoc-gen-go и Go runtime | v1.36.12 | BSD-3-Clause |
| connectrpc.com/connect/v2/cmd/protoc-gen-connect-go и runtime | v2.0.2 | Apache-2.0 |
| connectrpc.com/protoc-gen-connect-openapi | v0.30.0 | MIT |
| buf.build/go/protovalidate | v1.4.1 | Apache-2.0 |
| connectrpc.com/validate | v0.9.0 | Apache-2.0 |
| buf.build/bufbuild/protovalidate (схема) | v1.2.2, файл закоммичен в `api/third_party` | Apache-2.0 |
| @bufbuild/protoc-gen-es и @bufbuild/protobuf | 2.17.0 | Apache-2.0 |
| @bufbuild/protovalidate для контрактных проверок TS | 1.3.0 | Apache-2.0 |
| @connectrpc/connect и @connectrpc/connect-web | 2.3.0 | Apache-2.0 |

Go-зависимости и инструменты сохраняются в go.mod/go.sum, JS — точные версии и package-lock.json. Node 24.11.1 использован в исследовании. Для проекта сохраняется Go 1.27 из существующего go.mod. Полная сборка Go-генераторов и их совместная работа в проекте проверяются при реализации.

### Внешние proto без реестра

Реестр Buf в рабочем окружении недоступен (403 Forbidden), поэтому внешние схемы не подключаются remote-deps и `buf.lock` не используется. Необходимые файлы закоммичены в `api/third_party/proto` и объявлены локальным модулем `buf.yaml`:

| Файл | Источник | Закрепление | SHA-256 |
|---|---|---|---|
| `buf/validate/validate.proto` | `bufbuild/protovalidate` | тег `v1.2.2` | `a1dc1163ebac930b3b070655b91f2bc78df8b4aea940021661fe5ec7904a7b88` |
| `google/api/field_behavior.proto` | `googleapis/googleapis` | коммит `0616ac54ade8ad70502627e445fa786f93154459` | `e2a1ac88c49909d4579bacbb18519085e82da32c2c7bb15c31037f3be19f667f` |

Лицензии Apache-2.0 сохранены в `api/third_party/licenses`, происхождение и порядок обновления — в `api/third_party/README.md`. Импорты `google/protobuf/*.proto` предоставляет компилятор Buf. Замена не меняет проверки: `buf lint`, `buf build`, `buf breaking` и генерация работают локально, TypeScript собирается только из публичного каталога. Это отклонение от прежнего решения о `buf.lock` записано в таблицу отклонений.

Команды из корня

```text
npm --prefix api ci
npm --prefix api run generate
npm --prefix api run check
```

generate/check — короткая переносимая оркестрация готовых инструментов. Go tool запускается из контекста backend/go.mod, локальные JS-плагины — из api/node_modules. Рабочие каталоги и пути указываются явно в обёртке. Не требуется глобальная установка плагинов, shell make или генерация собственного компилятора/компаратора.

check включает buf format/lint/build, protovalidate-примеры, Go-сборку/тесты, TypeScript typecheck и повторную генерацию во временный каталог с обычным diff против отслеживаемых generated/golden файлов. Проверка не исправляет рабочие файлы. Для Empty допускаются узкие стандартные настройки lint вместо создания фиктивных пустых запросов.

Для совместимости используется `buf breaking` с категорией FILE против main после первого слияния контрактов. В этой первой ветке proto в main ещё нет: check явно сообщает отсутствие baseline и выполняет остальные проверки, не сравнивает схему с собой. После первого слияния отсутствие ожидаемой схемы в main — ошибка, а не вечное исключение. Выбор ref/main и подкаталога оформляется стандартным Git input Buf. При неполной локальной истории сначала получают main, не меняя рабочую ветку. Правила Buf не проверяют ужесточение произвольного CEL и все продуктовые ограничения, поэтому изменения аннотаций также видны в golden и проходят ревью. Новый собственный анализатор не нужен. Основание — [Buf breaking](https://buf.build/docs/breaking/).

### D7. Одноразовая сверка и golden

После генерации среза A выполняется одна ручная содержательная сверка исходного OpenAPI HLD с результатом. Проверяются все методы, поля, обязательность, форматы, диапазоны и массивы. Известные отличия сверяются с таблицей ниже. Имена схем могут измениться по правилам плагина; это не пропавшие DTO. Новый внутренний пакет, enum-префиксы, sizeBytes-number и нюансы null отмечаются явно.

Применяется готовый OpenAPI diff, если он поддерживает 3.1, либо обычный diff и ручная таблица сверки. Не требуется разработка программы распознавания эквивалентных схем. Результат сверки — текстовый отчёт в api/README или соседнем документе и утверждённые generated OpenAPI как golden. Далее check воспроизводит их и использует стандартное сравнение файлов. Обновление golden входит в ревью каждого изменения proto/base и не выполняется check автоматически. Отдельный дублирующий комплект схем не поддерживается.

### D8. Срез C — утверждённый вариант B Gateway

Владелец утвердил вариант B — полностью типизированный protobuf-протокол. Срез C включён в реализацию без дополнительного согласования формата. Приведённое ниже сравнение с исходным вариантом A из HLD обосновывает принятое решение и сохраняет результаты исследования.

В варианте B GatewayEnvelope содержит обязательный oneof payload из Identify, Ready, Heartbeat, HeartbeatAck, ServerEvent. Identify содержит token, Ready — userId, chatIds и readStates. ServerEvent сохраняет eventId и occurredAt, но вид события определяется обязательным oneof

- message_created и message_updated — Message.
- message_deleted — chatId, messageId, int32 revision.
- receipt_updated — ReceiptUpdated из D5.
- chat_changed — Chat.
- membership_changed — chatId и userId.
- profile_changed — отдельный PublicProfile с id/displayName/optional avatarAttachmentId, без identifier.
- presence_changed — Presence.

Вложенные модели используют те же proto-типы и валидатор, что RPC. У обеих oneof-групп действует required. В B отсутствуют числовой opcode, data_json, JSON в bytes payload, sequence и прежние резервные значения. Номера полей создаются заново для первого принятого контракта. Это осознанно несовместимая замена HLD, не попытка сохранить старый wire-формат.

| Критерий | A — HLD | B — типизированный |
|---|---|---|
| Клиент | Switch по opcode и имени события, дополнительный разбор JSON | Такой же switch по case, generated decoding, без второго JSON-разбора |
| Сложность | Оба варианта требуют heartbeat, авторизации и повторного чтения | Эти продуктовые алгоритмы остаются теми же, больший объём generated-типов не равен большей прикладной логике |
| Валидация | Protobuf проверяет оболочку, произвольный JSON нуждается в отдельной схеме | Структура событий и ограничения доступны protovalidate по дескриптору |
| Совместимость | buf breaking не видит структуру data_json | buf breaking видит поля событий; CEL и аудитории всё равно требуют тестов/ревью |
| Размер | Строковые имена и JSON-ключи внутри кадра | Обычно компактнее, преимущество зависит от текста сообщения |
| Переход | Соответствует исходным материалам | Меняются encoder/decoder Gateway и клиента; обновляется HLD §5.8 |

Измерены байты protobuf без WebSocket/TLS-заголовков и сжатия. Использованы одинаковые ULID, время и содержательные поля. Для большого сообщения сравнивался общий набор полей, без вложений и полной модели автора, поэтому это контрольный пример, не средний размер реального трафика.

| Контрольный кадр | A, байт | B, байт |
|---|---:|---:|
| Heartbeat | 2 | 2 |
| Identify, token 64 ASCII-символа | 80 | 68 |
| Ready, один chatId и пустые readStates | 100 | 58 |
| MessageDeleted | 166 | 98 |
| MessageCreated, текст 2048 ASCII-символов | 2366 | 2215 |

Выбран вариант B, поскольку нет выпущенных потребителей старого протокола, клиентская логика не усложняется, а структура становится доступной готовым средствам. Это не вводит replay и гарантированную доставку. Повторный Identify, интервалы 25/60 секунд, коды 4001/4003/4008 и перечитывание после READY сохраняются. Решение владельца закрывает последний открытый вопрос change.

### D9. Отклонения от HLD

| Что изменено | Где описано в HLD | Причина | Влияние на клиентов и карточки | Правка HLD |
|---|---|---|---|---|
| Proto — единственный источник; OpenAPI генерируется; ограничения protovalidate | §5.1–5.7, материалы openapi.json | Устранить независимые определения и использовать готовые инструменты | #6 меняет генерацию, карточки модулей подключают штатный валидатор | Да, источник и процесс контрактов |
| Значения enum получили префиксы, например CHAT_KIND_GROUP | §5.2–5.6, DTO и примеры | Настоящие типизированные enum и стандарт Buf | Клиент, Auth/Chat/Media/Presence и примеры используют generated enum; SQL-значения остаются прежними, маппинг в адаптерах | Да |
| int32 для ограниченных чисел; sizeBytes теперь JSON-number | §5.1–5.2, §5.6 | Размеры до 20 МиБ не требуют int64/строк, остальные поля укладываются в int32 | Клиент и Media меняют sizeBytes на number, проверяют преобразование из bigint; revision/counters имеют явную границу | Да |
| Timestamp/Empty — WKT; вход Timestamp допускает UTC offset, выход UTC | §5.1–5.2 | Стандартная сериализация и типы, меньше собственных DTO | Generated API использует WKT; JSON Empty остаётся {}, дата сохраняет значение | Уточнить WKT и допустимый вход |
| 5 внутренних RPC перенесены в molotalk.internal.v1 | §5.1, §5.3–5.6 | Структурная граница и отсутствие внутренних клиентов в TS | Новые внутренние пути для Gateway/AuthClient/ChatAccessClient/Presence; 29 внешних путей и Caddy не меняются | Да |
| Лимит сообщения 4096 вместо 4000 | §5.4 и материалы | Ранее принятое решение владельца | Chat, клиент и #5 должны использовать одинаковую границу, чужие файлы здесь не меняются | Да |
| Identifier lower-case и длины полей отражены в proto; пароль в контракте проверяется только по длине | §4.1–4.3, §5.2–5.3 | Согласовать формат с #5 и разделить формат/домен | Клиент нормализует identifier, Auth сохраняет проверку состава пароля и нормализацию; продуктовые правила пароля не отменены | Уточнить разделение обязанностей |
| ReceiptUpdated имеет два взаимоисключающих optional-поля и разные payload для автора/читателя | §5.8 | Явно выразить аудиторию и не раздавать список группе | Chat/Gateway/клиент используют receiptSummary либо собственный receipt, персональные отметки — GetMessageReceipts | Да, уточнить поля |
| Допускающие [] массивы помечены стандартной field_behavior REQUIRED; optional schema может допускать null | §5.1–5.2, обязательность в таблицах | Протокол protobuf не хранит presence repeated, генератор имеет ограничения null | Ответы сохраняют [] и не выдают null, runtime protovalidate строже документа для required; без собственного транслятора | Уточнить ограничения generated OpenAPI |
| Типизированный Gateway с oneof вместо смешанного protobuf/JSON — утверждённый вариант B | §5.8 и gateway.proto | Типизация, стандартная валидация и меньшие контрольные кадры | Одновременно меняются Gateway и клиент; формат среза C согласован | Да |
| Внешние proto закоммичены в `api/third_party` вместо `buf.lock` | §5.1–5.7 и порядок контрактов | Реестр Buf недоступен в рабочем окружении (403) | Генерация и проверки не требуют сети; происхождение и SHA-256 зафиксированы | Да, описание зависимостей |
| `required` в generated OpenAPI отражает только явные аннотации (`field_behavior`, `protovalidate.required`), а не все поля из HLD | §5.1–5.7, таблицы обязательности | Ограничение генератора: для proto3-скаляров без presence обязательность выражается правилами protovalidate | Клиенты получают более мягкие типы; сервер отклоняет пустые значения до вызова метода, расхождение перечислено в `api/openapi/comparison.md` | Уточнить при необходимости более строгих клиентов |

## Risks / Trade-offs

- Protovalidate не отменяет авторизацию и доменные правила. Состав пароля, права и аудитории проверяются модулями, а не HTTP-контроллером.
- Generated OpenAPI не исполняет произвольный CEL и может быть слабее runtime по null. Исключения перечислены, они не скрываются ручной правкой generated-файла.
- Base security содержит три явных исключения публичной авторизации. Он не превращается в ещё один каталог типов или список внутренней маршрутизации.
- Смена enum-строк и sizeBytes — несовместимые отличия материалов. Выпущенных клиентов нет, но все будущие потребители должны брать контракты из этой версии.
- Дополнение неизвестным событием не должно ронять будущий клиент. Политику игнорирования неизвестного case и последующего чтения реализует карточка клиента/Gateway.
- Одноразовая сверка требует содержательного ревью. Golden защищает от незамеченного изменения результата генерации, а не доказывает правильность бизнес-правил.

## Migration Plan

Срез A закрепляет инструментальную цепочку, публичные/внутренние proto, ограничения, OpenAPI и первую сверку/golden. Срез B проверяет транспорт, ошибки, сериализацию и воспроизводимость без прикладных сервисов. Срез C реализует только утверждённый контракт Gateway и проверки кодирования, не сетевой сервер.

До первого слияния отсутствие proto в main — документированный bootstrap-случай. После слияния buf breaking использует main. Перенос пользовательских данных не требуется. Для отмены до появления потребителей достаточно отката контрактов и зависимостей; после подключения клиентов изменения проводятся с проверкой совместимости.

## Open Questions

Открытых вопросов нет. Q1–Q4, политика ReceiptUpdated и вариант B для Gateway утверждены владельцем. Последующее обновление HLD отражает принятое решение и не блокирует реализацию контрактов в рамках этого change.
