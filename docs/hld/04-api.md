# 4 Спецификация API

<a name="section-4-1"></a>

## 4.1 Формат обмена и авторизация

Синхронный контракт использует unary Connect RPC поверх HTTPS [4]. Каждая операция вызывается POST по пути /molotalk.v1.&lt;Service&gt;/&lt;Method&gt;. Тело запроса и успешный ответ — JSON без дополнительного RPC-конверта. Заголовки: Content-Type: application/json и Connect-Protocol-Version: 1. Успех возвращается HTTP 200. Описание 34 операций и схем приведено в openapi.json в формате OpenAPI 3.1.

Защищённый запрос содержит Authorization: Bearer &lt;accessToken&gt;. Access token — JWT с sub (userId), sid (sessionId), exp, действует 15 минут; refresh token — 30 суток. Сервисы проверяют подпись JWT локально; отзыв блокирует refresh сразу, ранее выданный access token — не позднее exp. Register, Login и RefreshToken доступны без access token. Logout распространяет выход связанным вкладкам.

Внутренние методы доступны только доверенным сервисам и не маршрутизируются внешним прокси. Проверенный идентификатор пользователя передаётся в прикладную команду контекстом операции. Клиентский userId не используется как доказательство полномочий.

Поля HTTP JSON записываются lowerCamelCase. Id — ULID; Timestamp — UTC RFC 3339. int64, включая sizeBytes, передаётся десятичной строкой. Необязательное поле отсутствует, если не задано. В частичном обновлении отсутствие поля сохраняет значение; сброс аватара задаётся clearAvatar. Пустой запрос или результат — {}. X-Request-Id связывает запрос, ответ и журнал.

<a name="section-4-2"></a>

## 4.2 Общие структуры данных

Таблица 3 – Общие DTO

| DTO | Поля | Семантика |
| --- | --- | --- |
| User | id: Id<br>identifier?: string<br>displayName: string<br>avatarAttachmentId?: Id | Пароль и токены не входят. identifier (логин или e-mail) возвращается только самому пользователю в GetMe, UpdateProfile и SessionTokens. |
| SessionTokens | user: User<br>sessionId: Id<br>accessToken: string<br>refreshToken: string<br>accessExpiresAt: Timestamp<br>refreshExpiresAt: Timestamp | Результат регистрации, входа и ротации. user содержит собственный identifier; оба времени истечения обязательны. |
| Member | user: User<br>role: OWNER\|ADMIN\|MEMBER<br>joinedAt: Timestamp | OWNER вычисляется по ownerId; role определяет доступные действия. Данные User не содержат чужой identifier. |
| Chat | id: Id<br>kind: DIRECT\|GROUP<br>name?: string<br>ownerId?: Id<br>members: Member[]<br>unreadCount: int<br>lastMessageId?: Id<br>lastMessage?: Message<br>closedAt?: Timestamp | name и ownerId заданы только для GROUP. unreadCount совпадает с ReadState этого чата. lastMessage — последнее неудалённое сообщение. |
| Attachment | id: Id<br>purpose: AVATAR\|MESSAGE_ATTACHMENT<br>status: PENDING\|READY\|REJECTED<br>filename: string<br>contentType: string<br>sizeBytes: string<br>hasThumbnail: bool | PENDING — проверка не завершена; READY — файл готов; REJECTED — проверка не пройдена. hasThumbnail указывает наличие миниатюры. |
| Message | id: Id<br>chatId: Id<br>authorId: Id<br>clientMessageId: Id<br>text?: string<br>attachments: Attachment[]<br>attachmentsLinked: bool<br>revision: int<br>createdAt: Timestamp<br>editedAt?: Timestamp<br>deletedAt?: Timestamp<br>receiptSummary?: ReceiptSummary<br>author: User | author — публичный User без identifier, включая автора, уже вышедшего из чата. При deletedAt text отсутствует, attachments пусты; UI скрывает запись. attachmentsLinked отражает привязку, receiptSummary доступен автору. |
| ReceiptSummary | recipientCount: int<br>deliveredCount: int<br>readCount: int<br>state: ACCEPTED\|DELIVERED\|READ | Счётчики исходных получателей. DELIVERED/READ требуют ACK всех при N &gt; 0; при N = 0 остаётся ACCEPTED. |
| Receipt | messageId: Id<br>userId: Id<br>deliveredAt?: Timestamp<br>readAt?: Timestamp | Исходный получатель сообщения. deliveredAt и readAt отсутствуют до ACK; все устройства пользователя дают одну строку. |
| ReadState | chatId: Id<br>lastReadMessageId?: Id<br>unreadCount: int | Вычисляемое представление по message_receipts. Максимальный прочитанный id не означает прочтения всех предыдущих сообщений. |
| Presence | userId: Id<br>status: ONLINE\|OFFLINE | ONLINE при любом живом соединении пользователя; иначе OFFLINE. |

Знак ? в DTO означает необязательное поле. В каталогах методов звёздочкой отмечены обязательные поля запроса и результата; поле без звёздочки необязательно. История возвращает 50 записей по умолчанию, максимум 100. Превышение длины поля или размера массива — invalid_argument с именем поля в details. Специальные ограничения состояния, например 100 участников группы, возвращают failed_precondition.

<a name="section-4-3"></a>

## 4.3 Аккаунты и сеансы

Префикс методов: /molotalk.v1.AuthService/.

Таблица 4 – Методы AuthService

| Метод | Запрос | Ответ | Доступ и результат |
| --- | --- | --- | --- |
| Register | identifier*: string<br>displayName*: string<br>password*: string | tokens*: SessionTokens | Публичный. identifier — логин или e-mail до 254 символов, хранится в нижнем регистре и сравнивается без учёта регистра; displayName — 1–64 символа, не из одних пробелов. Атомарно создаёт уникальную учётную запись, учётные данные и первоначальный сеанс; пара возвращается после commit. Пароль проверяется по F_Pwd_1–3. Занятый identifier — already_exists. |
| Login | identifier*: string<br>password*: string | tokens*: SessionTokens | Публичный. Создаёт независимый сеанс для верных учётных данных. Неверная пара — unauthenticated без уточнения поля. После 5 неудачных попыток одного нормализованного identifier за 15 минут — resource_exhausted до конца окна; окно хранится в PostgreSQL. |
| RefreshToken | refreshToken*: string | tokens*: SessionTokens | Публичный. Атомарно погашает одноразовый refresh token, продлевает тот же сеанс на 30 суток от ротации и возвращает новую пару после commit. Повторное предъявление погашенного токена действующего сеанса отзывает этот сеанс и возвращает unauthenticated, в том числе после истечения исходного токена. Просроченный, неизвестный токен или неактивный сеанс — unauthenticated. |
| Logout | allSessions: bool | {} | Пользователь; sub и sid берутся из локально проверенного access JWT. Отзывает текущий sessionId, при allSessions = true — все сеансы. Gateway закрывает их WebSocket при уведомлении или проверке сеанса. Повтор не создаёт нового эффекта; access token действует до своего exp. |
| GetMe | {} | user*: User | Пользователь; sub берётся из локально проверенного access JWT. Возвращает текущий профиль, включая identifier. |
| UpdateProfile | displayName: string<br>avatarAttachmentId: Id<br>clearAvatar: bool | user*: User | Пользователь. Применяет переданные поля; имя — 1–64 символа, не из одних пробелов. Аватар должен быть READY, иметь purpose = AVATAR и принадлежать пользователю, иначе failed_precondition. clearAvatar = true удаляет аватар; одновременно с avatarAttachmentId — invalid_argument. |
| ChangePassword | currentPassword*: string<br>newPassword*: string<br>newPasswordConfirmation*: string | {} | Пользователь. Проверяет текущий пароль, совпадение новых полей и F_Pwd_1–3. Неверный текущий пароль — invalid_argument. После смены пароля остальные сеансы отзываются, текущий сохраняется. |
| SearchUsers | query*: string<br>limit: int | users*: User[] | Пользователь. query — 2–100 символов; по имени ищется начало слова, по identifier — только полное совпадение. limit — 20 по умолчанию, максимум 50. Текущий пользователь исключён; identifier других пользователей не возвращается. |
| IntrospectSession | sessionId*: Id<br>userId*: Id | active*: bool | Внутренний. Проверяет существование, срок и отсутствие отзыва сеанса, его принадлежность userId. sid и sub берутся из проверенного access token. Вызывается Gateway при IDENTIFY и перед выдачей событий. |

<a name="section-4-4"></a>

## 4.4 Диалоги, сообщения и подтверждения

Префикс методов: /molotalk.v1.ChatService/.

Таблица 5 – Методы ChatService

| Метод | Запрос | Ответ | Доступ и результат |
| --- | --- | --- | --- |
| ListChats | cursor: string<br>limit: int | chats*: Chat[]<br>nextCursor: string | Пользователь. Возвращает доступные диалоги с участниками, ролями и счётчиками, по убыванию времени последнего неудалённого сообщения; для пустых — времени создания. Равенство разрешается id. Страница — 50, максимум 100. nextCursor задан при следующей странице. |
| GetChat | chatId*: Id | chat*: Chat | Пользователь. Возвращает актуальную карточку доступного диалога. |
| CreateDirectChat | peerUserId*: Id | chat*: Chat | Пользователь. Возвращает существующий диалог пары либо создаёт его; повтор не создаёт второй диалог. peerUserId не равен собственному id; неизвестный пользователь — not_found. |
| CreateGroupChat | clientRequestId*: Id<br>name*: string<br>memberIds: Id[] | chat*: Chat | Пользователь. Создаёт группу и состав локальной транзакцией; создатель — владелец и входит в лимит 100. name — 1–100 символов, не из одних пробелов. Повторы и id создателя в memberIds игнорируются. Более 99 других участников — failed_precondition, неизвестный пользователь — not_found. Тот же clientRequestId и состав возвращают прежнюю группу; иная команда с тем же ключом — already_exists. |
| AddGroupMember | chatId*: Id<br>userId*: Id | chat*: Chat | Пользователь. Доступен владельцу/администратору GROUP. Проверка лимита и добавление сериализуются по группе. При 100 участниках — failed_precondition. Добавление текущего участника возвращает актуальную карточку без изменения состава. |
| RemoveGroupMember | chatId*: Id<br>userId*: Id | chat*: Chat | Пользователь. Владелец удаляет другого участника; администратор — только MEMBER, иначе permission_denied. Удаление себя — invalid_argument: используется LeaveGroup. Владелец не исключается этим методом. |
| LeaveGroup | chatId*: Id | closed*: bool<br>newOwnerId: Id | Пользователь. Только GROUP, для DIRECT — failed_precondition. Удаляет текущего участника. Выход владельца передаёт владение администратору, а при их отсутствии — случайному участнику в той же транзакции; newOwnerId задан только при передаче. Пустая группа закрывается. |
| SetGroupMemberRole | chatId*: Id<br>userId*: Id<br>role*: ADMIN\|MEMBER | chat*: Chat | Пользователь. Только владелец назначает/снимает администратора; владение этим методом не передаётся. Изменение собственной роли — invalid_argument; userId вне состава — not_found. |
| SendMessage | chatId*: Id<br>clientMessageId*: Id<br>text: string<br>attachmentIds: Id[] | message*: Message | Пользователь. Сохраняет непустой текст до 4000 символов либо 1–5 готовых файлов. Вместе с сообщением фиксирует строки получателей из текущего состава без автора. Тот же ключ и исходная команда дают тот же id; другая команда — already_exists. После записи отдельно привязывает файлы и публикует событие; сбой привязки отражает attachmentsLinked = false. |
| EditMessage | chatId*: Id<br>messageId*: Id<br>expectedRevision*: int<br>text: string | message*: Message | Пользователь. Проверяет членство и авторство; меняет только текст. Отсутствующий text не меняет сообщение, пустой допустим при вложениях. Устаревшая expectedRevision — aborted, удалённое сообщение — failed_precondition. При изменении повышает revision и выставляет editedAt. |
| DeleteMessage | chatId*: Id<br>messageId*: Id | messageId*: Id<br>revision*: int | Пользователь. Мягко удаляет собственное сообщение. Повтор идемпотентен. История возвращает маркер deletedAt без содержимого, UI скрывает запись. Новые URL вложений не выдаются, прежние действуют до истечения. Участникам направляется MessageDeleted. |
| GetMessages | chatId*: Id<br>mode: LATEST\|BEFORE\|AFTER\|RANGE<br>cursor: string<br>fromMessageId: Id<br>toMessageId: Id<br>limit: int | messages*: Message[]<br>hasMore*: bool<br>nextCursor: string | Пользователь. LATEST — по умолчанию; BEFORE возвращает id &lt; toMessageId, AFTER — id &gt; fromMessageId, RANGE — обе включительные границы. limit — 50, максимум 100. Результат всегда по возрастанию id, включая маркеры deletedAt. При hasMore = true задан nextCursor. С cursor передаются только chatId и limit; режим и границы сохранены в курсоре. Окно заменяется после чтения всего диапазона. |
| ConfirmDelivery | chatId*: Id<br>messageIds*: Id[] | {} | Пользователь. Подтверждает 1–100 сообщений одного доступного чата. Обновляются только собственные строки message_receipts; остальные id пропускаются, при отсутствии подходящих — permission_denied. Первый deliveredAt сохраняется, повтор безопасен. |
| MarkRead | chatId*: Id<br>messageIds*: Id[] | readState*: ReadState | Пользователь. Подтверждает 1–100 показанных сообщений активного видимого чата по тем же правилам отбора. readAt заполняется однократно, одновременно обеспечивается deliveredAt. Возвращает вычисленный ReadState; максимум подтверждённых id не означает прочтения пропущенных сообщений. |
| GetMessageReceipts | chatId*: Id<br>messageId*: Id | summary*: ReceiptSummary<br>receipts*: Receipt[] | Пользователь. Детальный список исходных получателей и времён ACK выдаётся автору при текущем доступе к чату. До ACK времена отсутствуют. Другой участник — permission_denied; без доступа — not_found. |
| GetReadStates | {} | states*: ReadState[] | Пользователь. Возвращает вычисляемые ReadState доступных чатов: непрочитанные неудалённые сообщения и максимальный подтверждённый id. Отдельной таблицы состояния чтения нет. |
| CheckChatAccess | userId*: Id<br>chatId*: Id<br>attachmentId: Id | allowed*: bool | Внутренний. Проверяет текущее членство; при attachmentId — также ссылку из неудалённого сообщения и завершённую привязку файла к этому чату. |
| GetUserChatIds | userId*: Id | chatIds*: Id[]<br>readStates*: ReadState[] | Внутренний. Вызывается Gateway при подключении; chatIds определяют подписки и аудиторию присутствия, readStates вычисляются для READY. |

<a name="section-4-5"></a>

## 4.5 Загрузка и доступ к файлам

Префикс методов: /molotalk.v1.MediaService/.

Таблица 6 – Методы MediaService

| Метод | Запрос | Ответ | Доступ и результат |
| --- | --- | --- | --- |
| PrepareUpload | purpose*: AVATAR\|MESSAGE_ATTACHMENT<br>filename*: string<br>contentType*: string<br>sizeBytes*: string | attachmentId*: Id<br>uploadUrl*: string<br>uploadExpiresAt*: Timestamp<br>requiredHeaders*: map&lt;string, string&gt; | Пользователь. Проверяет назначение, заявленные формат и размер; filename — 1–255 символов без /, обратной косой черты и управляющих символов. Выдаёт presigned PUT на 15 минут с подписанным Content-Type. Браузер передаёт разрешённые requiredHeaders и File напрямую в S3. Content-Length устанавливает браузер; фактический размер обязательно проверяется в CompleteUpload. |
| CompleteUpload | attachmentId*: Id | attachment*: Attachment | Пользователь. Проверяет владельца, фактический размер и формат; создаёт миниатюру изображения, переносит проверенные байты на финальный ключ и возвращает READY. Отсутствие объекта — failed_precondition; неверный формат/размер — invalid_argument, REJECTED и удаление временного объекта. Повтор READY идемпотентен. |
| GetAttachment | attachmentId*: Id | attachment*: Attachment | Пользователь. Выдаёт метаданные по тем же правилам видимости, что GetDownloadURL. Незакреплённая загрузка видна только владельцу; PENDING объединяет UPLOADING и VALIDATING. DELETING не выдаётся как доступный объект. |
| GetDownloadURL | attachmentId*: Id<br>thumbnail: bool | url*: string<br>expiresAt*: Timestamp | Пользователь. Выдаёт presigned GET на час. Аватар доступен по текущей ссылке профиля; вложение — текущему участнику при ссылке из неудалённого сообщения и привязке к чату. Владелец может просматривать собственный READY до привязки. Нет файла/права — not_found; доступный файл не READY, его привязка не завершена либо миниатюры нет — failed_precondition. Для изображений Content-Disposition: inline, для документов — attachment с исходным именем. Байты передаются из S3. |

<a name="section-4-6"></a>

## 4.6 Присутствие

Префикс методов: /molotalk.v1.PresenceService/.

Таблица 7 – Методы PresenceService

| Метод | Запрос | Ответ | Доступ и результат |
| --- | --- | --- | --- |
| GetPresence | userIds*: Id[] | presences*: Presence[] | Пользователь. userIds — 1–200. Возвращает статусы собеседников и общих участников; остальные id пропускаются без ошибки. |
| Heartbeat | userId*: Id<br>sessionId*: Id<br>connectionId*: Id | {} | Внутренний. Вызывается Gateway на HEARTBEAT клиента. Продлевает TTL соединения Redis до 60 секунд; время задаёт сервер. Статус ONLINE, пока существует хотя бы одно живое соединение. |
| Disconnect | userId*: Id<br>sessionId*: Id<br>connectionId*: Id | {} | Внутренний. Вызывается Gateway при закрытии соединения. Удаляет одно соединение и пересчитывает присутствие; при потере вызова запись истекает по TTL. |

<a name="section-4-7"></a>

## 4.7 Правила операций и обработка ошибок

Register и ChangePassword применяют одну политику: 6–64 символа, минимум одна заглавная латинская буква, одна строчная, цифра и специальный символ. Разрешены A–Z, a–z, 0–9 и ! @ # $ % ^ &amp; * ( ) _ + - = [ ] { } ; : ' " | , . &lt; &gt; / ?. Пробелы, кириллица, emoji, управляющие символы, обратная косая черта и остальные символы отклоняются. Сервер также проверяет совпадение newPassword и newPasswordConfirmation.

SendMessage принимает непустой после проверки пробелов текст до 4000 символов либо 1–5 готовых файлов. Ключ (chatId, authorId, clientMessageId) и отпечаток команды определяют повтор: прежняя команда возвращает сохранённое сообщение, иная — already_exists. Повтор после удаления не восстанавливает запись. Сообщение и строки получателей записываются локальной транзакцией Chat с согласованным составом; привязка вложений выполняется отдельно. Её ошибка не отменяет принятие сообщения и отражается attachmentsLinked = false.

GetMessages поддерживает LATEST по умолчанию, BEFORE (id &lt; toMessageId), AFTER (id &gt; fromMessageId) и RANGE с двумя включительными границами. Для продолжения используется cursor с теми же chatId и limit; режим и границы повторно не передаются. Страницы возвращаются по возрастанию id, включая маркеры удаления. Окно заменяется после чтения всех страниц диапазона; события за время чтения согласуются по id и revision. Маркеры deletedAt скрываются в интерфейсе.

ACK изменяет только строку получателя с текущим доступом; остальные id пропускаются, при отсутствии подходящих возвращается permission_denied. MarkRead вызывается для показанных сообщений активного видимого чата и подразумевает получение. Новые участники не меняют исторический recipientCount. Подробные receipts доступны автору. ReadState вычисляется по подтверждениям. Публикация NATS не является подтверждением доставки браузеру или прочтения человеком.

PrepareUpload проверяет заявленные параметры, CompleteUpload — реальные байты и формат. AVATAR допускает JPEG/PNG до 5242880 байт (5 МБ), MESSAGE_ATTACHMENT — JPEG/PNG/GIF/PDF/DOCX до 20971520 байт (20 МБ). Браузер выполняет прямой PUT на 15 минут с согласованным Content-Type; Content-Length управляется браузером. Media проверяет фактический размер до READY. Затем файл связывается с профилем или сообщением. GetDownloadURL выдаёт GET на один час после проверки доступа; для браузерного PUT/GET в S3 настроен CORS.

Таблица 8 – Единая модель ошибок

| HTTP | Код Connect | Условие |
| --- | --- | --- |
| 400 | invalid_argument | Пустое/слишком длинное поле, неверный формат или размер, нарушение политики пароля, несовпадение новых паролей либо неверный текущий пароль. |
| 400 | failed_precondition | Файл не готов, достигнут предел 100 участников либо операция несовместима с состоянием. |
| 401 | unauthenticated | Отозванный/истёкший сеанс, просроченный токен либо неверные учётные данные при входе. |
| 403 | permission_denied | Участник чата не имеет необходимой роли, авторства или права подтверждать сообщение. |
| 404 | not_found | Сущность отсутствует либо текущий пользователь не имеет доступа; существование чужих объектов не раскрывается. |
| 409 | already_exists | Занятый identifier либо конфликт ключа повторного создания группы/отправки сообщения. |
| 409 | aborted | Устаревшая expectedRevision при EditMessage. |
| 429 | resource_exhausted | Превышена частота запросов, включая неудачные попытки входа. |
| 503 | unavailable | Недоступна обязательная зависимость операции. |
| 500 | internal | Непредвиденная ошибка обработки. |

Ошибка содержит code, message и необязательные details. Клиент не обновляет токен по unauthenticated от Login, Register или RefreshToken. Ошибка чтения не трактуется как пустая история.

POST /molotalk.v1.ChatService/SendMessage

Content-Type: application/json

Connect-Protocol-Version: 1

Authorization: Bearer &lt;accessToken&gt;

{ "chatId": "01K7ZJ8N40N000000000000001",

  "clientMessageId": "01K7ZJ8N40N000000000000010",

  "text": "Привет", "attachmentIds": [] }

Успешный HTTP 200 содержит поле message с DTO Message: id, chatId, authorId, clientMessageId, text, attachments, attachmentsLinked, revision = 1 и createdAt. Например, отказ серверной проверки имеет вид:

{ "code": "invalid_argument",

  "message": "Нельзя отправить пустое сообщение без файлов" }

<a name="section-4-8"></a>

## 4.8 Канал событий WebSocket

Клиент открывает WSS /ws через входной прокси. Передаётся IDENTIFY с JSON {"token":"&lt;accessToken&gt;"}; токен не включается в URL. Перед истечением access token клиент повторяет IDENTIFY новым токеном того же сеанса. Gateway закрывает соединение кодом 4001 при ошибке/истечении авторизации, 4003 при отзыве сеанса, 4008 при отсутствии HEARTBEAT дольше 60 секунд. После 4001 выполняется refresh и переподключение, после 4003 — переход ко входу, после 4008 — переподключение с обычным чтением состояния.

Таблица 9 – Служебные кадры WebSocket

| Кадр | Направление | Назначение |
| --- | --- | --- |
| IDENTIFY | Клиент → Gateway | Первичная авторизация или продление новым access token того же сеанса. |
| READY | Gateway → клиент | JSON userId, chatIds, readStates; затем запускается чтение API. readStates вычислены по receipts. |
| HEARTBEAT | Клиент → Gateway | Подтверждение активности по умолчанию раз в 25 секунд. |
| HEARTBEAT_ACK | Gateway → клиент | Подтверждение heartbeat. |
| EVENT | Gateway → клиент | Protobuf ServerEvent: eventId, type, occurredAt, data_json с JSON DTO. |

События: MessageCreated/MessageUpdated передают Message; MessageDeleted — chatId, messageId, revision; ReceiptUpdated — messageId и актуальные данные для автора и сеансов читателя; ChatChanged — Chat; MembershipChanged — chatId и userId затронутого участника; ProfileChanged — User без чужого identifier; PresenceChanged — Presence. Изменения состава адресуются текущим участникам и персонально исключённому пользователю. Полные receipts не рассылаются группе. eventId позволяет отбрасывать дубликаты; неподходящая revision не перезаписывает более новое состояние.

Gateway подписывается на NATS отдельно в каждом экземпляре, проверяет действительность сеанса и актуальную аудиторию перед передачей кадра. Дубликаты и устаревшие revision не меняют представление. После обрыва или пропуска уведомления клиент не запрашивает replay: он получает READY и перечитывает текущее состояние. Heartbeat и интервалы обновления являются конфигурацией, доступной для подмены в тестах.
