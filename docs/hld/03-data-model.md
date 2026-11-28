# 3 Модель данных

<a name="section-3-1"></a>

## 3.1 Состав и владение данными

Постоянное состояние системы хранится в единой реляционной БД PostgreSQL, что обеспечивает строгие связи и целостность данных. При этом логическое владение таблицами разделено между сервисами:

• Auth - управляет пользователями, паролями, сеансами и ссылками на аватары;

• Chat - владеет данными диалогов, сообщений и подтверждений;

• Media - отвечает за состояние и атрибуты загрузок файлов.

Сервисы изолированы на уровне доступа к БД: каждый процесс использует собственный пул соединений. Важное ограничение - SQL-транзакции не передаются между сервисами, поэтому атомарность гарантируется только в рамках бизнес-операции одного сервиса, а взаимодействие строится через явные контракты. Например, при отправке вложения сервис Chat отдельно заполняет поля channel_id и linked_at в таблице файлов.

Модель включает 10 таблиц. Для распределенной среды применяются единые стандарты:

• Идентификаторы - первичные ключи представлены ULID (26 символов). Они лексикографически сортируемы и генерируются независимо; уникальность проверяется ограничениями БД.

• Время - хранится в формате timestamptz. Время формируется прикладными часами сервиса, а не напрямую СУБД, что обеспечивает тестируемость.

Хранилища Redis и S3 не входят в реляционную ERD, так как решают отдельные задачи: Redis содержит временные соединения присутствия (с TTL), а S3 - бинарные объекты по ключам из таблицы attachments.

<a name="section-3-2"></a>

## 3.2 Связи сущностей

Единая ERD содержит все 10 таблиц и их поля. PK обозначает первичный ключ, FK — внешний, UQ — уникальность. Звёздочкой отмечены обязательные поля. Остальные допускают NULL. Тип ulid соответствует char(26) в словаре атрибутов и домену ulid в SQL. Ссылки массива attachment_ids являются логическими связями.

Пользователь связан с учётными данными и независимыми сеансами. Refresh tokens принадлежат сеансу; consumed_at отмечает погашение. Хеш погашенного токена хранится до истечения сеанса. login_attempt_windows хранит счётчик и начало окна попыток входа; identifier_hash вычисляется из нормализованного identifier. avatar_attachment_id ссылается на attachments.id.

channels задаёт тип диалога, владельца, исходного создателя и ключ повторного создания группы. created_by_id и owner_id ссылаются на users.id. channel_members связывает диалог с пользователем; его PK — пара (channel_id, user_id). Составной отложенный FK (channels.id, owner_id) ссылается на эту пару. Роль OWNER вычисляется по owner_id; поле role содержит ADMIN или MEMBER. Уникальны direct_pair_key и пара (created_by_id, client_request_id).

Сообщение связано с channels.id и users.id через channel_id и author_id. UQ* обозначает составную уникальность (channel_id, author_id, client_message_id). Для каждого участника на момент отправки, кроме автора, создаётся строка message_receipts с ключом (message_id, user_id); времена ACK первоначально NULL. read_at требует заполненного delivered_at. ReadState и unreadCount вычисляются запросами по этим строкам и неудалённым сообщениям. Массив attachment_ids содержит до пяти логических ссылок на attachments.id; поэлементный FK не объявляется.

uploader_id ссылается на users.id, channel_id — на channels.id. object_key и temp_object_key уникальны. channel_id и linked_at заполняются совместно отдельным UPDATE. Внутренние состояния UPLOADING и VALIDATING отображаются в API как PENDING; READY и REJECTED сохраняют названия, DELETING является внутренним состоянием очистки. hasThumbnail вычисляется по thumbnail_key, attachmentsLinked — по фактическим привязкам файлов сообщения.

<a name="section-3-3"></a>

## 3.3 Атрибуты и ограничения

Таблица 2 – Атрибуты сущностей

| Сущность | Поля и типы | Ключи и правила |
| --- | --- | --- |
| users<br>Пользователи | id : char(26)<br>identifier : text<br>display_name : text<br>avatar_attachment_id : char(26) NULL<br>created_at : timestamptz | Уникальны identifier и непустая ссылка аватара. identifier — логин или e-mail, хранится в нижнем регистре; display_name — 1–64 символа, не из одних пробелов. |
| user_auth<br>Пароли | user_id : char(26)<br>password_hash : text<br>password_changed_at : timestamptz | PK одновременно FK на пользователя. Сохраняются хеш с параметрами проверки и временем изменения; открытый пароль не хранится. |
| sessions<br>Сеансы | id : char(26)<br>user_id : char(26)<br>created_at : timestamptz<br>expires_at : timestamptz<br>revoked_at : timestamptz NULL | Независимый сеанс пользователя; срок проверяется сервером. Успешная ротация refresh продлевает тот же сеанс на 30 суток; revoked_at отмечает отзыв. |
| refresh_tokens<br>Обновление сеанса | token_hash : text<br>session_id : char(26)<br>expires_at : timestamptz<br>consumed_at : timestamptz NULL | PK — хеш токена. consumed_at заполняется при ротации; новый хеш и продление сеанса фиксируются в той же транзакции. Повтор погашенного токена отзывает связанный сеанс; запись сохраняется до окончания сеанса. |
| login_attempt_windows<br>Окна попыток входа | identifier_hash : char(64)<br>failed_count : smallint<br>window_started_at : timestamptz | PK — SHA-256 нормализованного identifier. failed_count — 1–5; окно — 15 минут. Проверка и изменение счётчика сериализуются по ключу; истёкшие окна очищаются. |
| channels<br>Диалоги | id : char(26)<br>created_by_id : char(26)<br>client_request_id : char(26) NULL<br>create_request_hash : text NULL<br>kind : text<br>name : text NULL<br>owner_id : char(26) NULL<br>direct_pair_key : text NULL<br>created_at : timestamptz<br>closed_at : timestamptz NULL | Уникальны пара DIRECT и (created_by_id, client_request_id). Для GROUP ключ запроса и его хеш обязательны. Открытая группа имеет имя и владельца; принадлежность владельца составу проверяет отложенный FK. |
| channel_members<br>Участие | channel_id : char(26)<br>user_id : char(26)<br>role : text<br>joined_at : timestamptz | PK (channel_id, user_id). ADMIN/MEMBER; OWNER определяется channels.owner_id. Добавление, выход и передача владения блокируют строку группы. |
| messages<br>Сообщения | id : char(26)<br>channel_id : char(26)<br>author_id : char(26)<br>client_message_id : char(26)<br>text : text<br>attachment_ids : varchar(26)[]<br>revision : integer<br>created_at : timestamptz<br>edited_at : timestamptz NULL<br>deleted_at : timestamptz NULL<br>request_hash : text | Уникален (channel_id, author_id, client_message_id). text — до 4000 символов; до пяти файлов. revision начинается с 1; request_hash — отпечаток исходной команды. |
| message_receipts<br>Подтверждения | message_id : char(26)<br>user_id : char(26)<br>delivered_at : timestamptz NULL<br>read_at : timestamptz NULL | PK (message_id, user_id). Строка для каждого исходного получателя. До ACK времена NULL; read_at требует delivered_at и не может быть раньше. Подтверждения монотонны. |
| attachments<br>Файлы | id : char(26)<br>uploader_id : char(26)<br>purpose : text<br>status : text<br>object_key : text<br>temp_object_key : text NULL<br>thumbnail_key : text NULL<br>filename : text<br>content_type : text<br>size_bytes : bigint<br>channel_id : char(26) NULL<br>linked_at : timestamptz NULL<br>created_at : timestamptz<br>upload_expires_at : timestamptz<br>completed_at : timestamptz NULL | Уникальны object_key и temp_object_key. READY следует после проверки. channel_id и linked_at заполняются вместе отдельным UPDATE; размеры ограничены 5/20 МБ. |

История индексируется по (channel_id, id), участие и подтверждения — по user_id. Для непрочитанных строк message_receipts задан частичный индекс, для проверки ссылок attachment_ids — GIN. История и подтверждения сохраняются после выхода участника.

Изменение состава блокирует строку группы: предел 100 и добавление проверяются одной транзакцией. Выход владельца и передача владения атомарны.

После проверки объект переносится на финальный ключ, для которого PUT не выдавался. Выданный GET действует до истечения срока.
