workspace "MoloTalk" "Архитектура системы текстового общения." {
    !identifiers flat
    model {
        !impliedRelationships false
        user = person "Пользователь" "Человек, которому нужно поддерживать связь с собеседниками."
        system = softwareSystem "MoloTalk" "Обеспечивает текстовое общение один на один и в группах." {
            web = container "Веб-клиент" "Отображает данные и преобразует действия пользователя в запросы к системе." "TypeScript, React" {
                tags "Browser"
                accessUi = component "Вход и профиль" "Связывает поля форм с проверками ввода и отображает ошибки аккаунта." "React"
                chatUi = component "Диалоги и группы" "Управляет выбором диалога и формами изменения состава и ролей." "React"
                composer = component "Редактор сообщений" "Ведёт черновик, список загрузок и очередь повторной отправки." "React"
                timeline = component "История и статусы" "Управляет окнами сообщений, положением прокрутки и индикаторами подтверждений." "React"
                sync = component "Обмен данными" "Преобразует ответы API и события в изменения состояния; объединяет параллельные запросы." "TypeScript"
                localState = component "Состояние клиента" "Хранит нормализованные сущности и сопоставляет локальные записи с подтверждёнными." "TypeScript"
            }
            edge = container "Входной прокси" "Раздаёт веб-клиент и направляет запросы API и соединения событий." "Caddy" {
                tags "Edge"
            }
            auth = container "Сервис аккаунтов" "Управляет учётными записями и жизненным циклом сеансов." "Go" {
                tags "Service"
                authApi = component "API аккаунтов" "Преобразует RPC-запросы в команды и сопоставляет ошибки с ответами API." "Go"
                passwords = component "Правила паролей" "Проверка длины, разрешённых символов и обязательных классов." "Go"
                credentials = component "Проверка пароля" "Серверное хеширование паролей и проверка текущего пароля." "Go"
                sessions = component "Управление сеансами" "Выпуск, ротация и отзыв токенов; проверка действующего сеанса." "Go"
                accounts = component "Аккаунты и профиль" "Нормализует идентификатор, применяет изменения атрибутов и проверяет ссылку на аватар." "Go"
                authRepo = component "Хранилище аккаунтов" "Выполняет SQL-запросы пользователей и сеансов, преобразует результаты в модели." "Go"
            }
            chat = container "Сервис чатов" "Применяет правила участия в переписке и сохраняет состояние диалогов." "Go" {
                tags "Service"
                chatApi = component "API чатов" "Разбирает команды и параметры выборок, передаёт их доменным обработчикам." "Go"
                access = component "Проверка доступа" "Членство в чате, роль в группе и авторство сообщения." "Go"
                groups = component "Управление группами" "Проверяет предел состава, изменяет роли и выбирает преемника владельца." "Go"
                messages = component "Обработка сообщений" "Сопоставляет ключ повтора с записью, проверяет авторство и повышает версию изменения." "Go"
                receipts = component "Получение и прочтение" "Объединяет подтверждения по получателю и вычисляет итоговый индикатор." "Go"
                queries = component "Чтение состояния" "Собирает страницы по курсору и представления диалога с участниками и статусами." "Go"
                chatRepo = component "Хранилище чатов" "Выполняет выборки и изменения SQL, преобразует строки в модели переписки." "Go"
            }
            media = container "Сервис файлов" "Управляет загрузками и выдаёт разрешённый доступ к объектам." "Go" {
                tags "Service"
                mediaApi = component "API файлов" "Разбирает параметры объекта и направляет команды загрузки или запроса доступа." "Go"
                filePolicy = component "Проверка файла" "Формат и фактический размер; отдельные правила для аватара и вложения." "Go"
                assets = component "Управление файлами" "Координирует переходы загрузки от подготовки к проверке, готовности и очистке." "Go"
                fileAccess = component "Доступ к файлам" "Проверяет владельца загрузки, профиль или доступ к связанному чату перед выдачей ссылки." "Go"
                objectAdapter = component "Доступ к S3" "Подписывает операции объекта, читает его атрибуты и выполняет перенос или удаление." "Go"
                mediaRepo = component "Метаданные файлов" "Сохраняет состояние загрузки, владельца, назначение и ключ объекта." "Go"
            }
            presence = container "Сервис присутствия" "Вычисляет доступность пользователя по его подключениям." "Go" {
                tags "Service"
                presenceApi = component "API присутствия" "Разбирает сигналы соединений и запросы списка наблюдаемых пользователей." "Go"
                leases = component "Активные подключения" "Продлевает TTL каждого соединения и агрегирует несколько соединений одного пользователя." "Go"
                visibility = component "Видимость статусов" "Определяет собеседников и участников общих чатов." "Go"
                presenceEvents = component "События присутствия" "Формирует уведомление при смене агрегированного состояния пользователя." "Go"
            }
            gateway = container "Шлюз событий" "Обеспечивает обмен событиями с подключёнными клиентами." "Go" {
                tags "Service"
                ws = component "WebSocket-транспорт" "Разбирает и кодирует кадры, ведёт heartbeat и управляет буферами подключения." "Go"
                sessionGuard = component "Проверка сеанса" "Проверяет токен и активность сеанса подключения." "Go"
                subscriptions = component "Подписки клиентов" "Сопоставляет соединения с членством в чатах и выбирает адресатов по chat_id." "Go"
                dispatch = component "Рассылка событий" "Декодирует уведомления брокера и направляет их в транспорт выбранных соединений." "Go"
                recovery = component "Инициализация подключения" "Собирает READY из доступных подписок и сохранённых отметок прочтения." "Go"
            }
            db = container "База данных" "Хранит постоянное состояние системы и обеспечивает ограничения целостности." "PostgreSQL" {
                tags "Database"
            }
            redis = container "Хранилище присутствия" "Активные подключения и сроки их действия." "Redis" {
                tags "Cache"
            }
            bus = container "Брокер событий" "Связывает источники изменений с подписчиками событий." "NATS Core" {
                tags "Queue"
            }
            objects = container "Хранилище файлов" "Хранит содержимое загруженных объектов и миниатюр." "S3" {
                tags "ObjectStore"
            }
        }
        user -> system "Ведёт переписку" "Веб-интерфейс" {
            tags "Interaction"
        }
        user -> web "Вводит данные и выполняет действия интерфейса" "Веб-интерфейс" {
            tags "Interaction"
        }
        web -> edge "Вызывает API и открывает канал событий" "HTTPS / WSS" {
            tags "Sync"
        }
        edge -> auth "Вызывает API аккаунтов" "ConnectRPC" {
            tags "Sync"
        }
        edge -> chat "Вызывает API переписки" "ConnectRPC" {
            tags "Sync"
        }
        edge -> media "Вызывает API файлов" "ConnectRPC" {
            tags "Sync"
        }
        edge -> presence "Запрашивает статусы" "ConnectRPC" {
            tags "Sync"
        }
        edge -> gateway "Открывает соединения клиентов" "WebSocket" {
            tags "Sync"
        }
        chat -> auth "Проверяет сеанс" "ConnectRPC" {
            tags "Sync"
        }
        media -> auth "Проверяет сеанс" "ConnectRPC" {
            tags "Sync"
        }
        presence -> auth "Проверяет сеанс" "ConnectRPC" {
            tags "Sync"
        }
        gateway -> auth "Проверяет сеанс" "ConnectRPC" {
            tags "Sync"
        }
        auth -> db "Хранит аккаунты и сеансы" "SQL" {
            tags "Data"
        }
        chat -> db "Хранит переписку" "SQL" {
            tags "Data"
        }
        media -> db "Хранит метаданные" "SQL" {
            tags "Data"
        }
        media -> objects "Проверяет объекты" "S3 API / HTTPS" {
            tags "Data"
        }
        web -> objects "Загружает и скачивает файлы" "HTTPS PUT / GET" {
            tags "Data"
        }
        media -> chat "Проверяет доступ к файлу" "ConnectRPC" {
            tags "Sync"
        }
        presence -> chat "Определяет видимость" "ConnectRPC" {
            tags "Sync"
        }
        presence -> redis "Учитывает подключения" "RESP" {
            tags "Data"
        }
        gateway -> chat "Читает подписки и аудиторию" "ConnectRPC" {
            tags "Sync"
        }
        gateway -> presence "Передаёт heartbeat" "ConnectRPC" {
            tags "Sync"
        }
        auth -> bus "Публикует события аккаунтов" "NATS publish" {
            tags "Async"
        }
        chat -> bus "Публикует события переписки" "NATS publish" {
            tags "Async"
        }
        presence -> bus "Публикует присутствие" "NATS publish" {
            tags "Async"
        }
        gateway -> bus "Подписывается на события" "NATS subscribe" {
            tags "Async"
        }
        user -> accessUi "Заполняет формы аккаунта" "Веб-интерфейс" {
            tags "Interaction"
        }
        accessUi -> sync "Передаёт команды аккаунта" "Вызов функции" {
            tags "Sync"
        }
        accessUi -> localState "Получает профиль и сеанс" "Вызов функции" {
            tags "Sync"
        }
        user -> chatUi "Выбирает диалог и управляет группой" "Веб-интерфейс" {
            tags "Interaction"
        }
        chatUi -> sync "Запрашивает диалоги и меняет состав" "Вызов функции" {
            tags "Sync"
        }
        chatUi -> localState "Получает выбранный чат и участников" "Вызов функции" {
            tags "Sync"
        }
        user -> composer "Готовит и отправляет сообщение" "Веб-интерфейс" {
            tags "Interaction"
        }
        composer -> sync "Передаёт сообщение и загрузки" "Вызов функции" {
            tags "Sync"
        }
        composer -> localState "Сохраняет черновики и очередь" "Вызов функции" {
            tags "Sync"
        }
        user -> timeline "Читает историю и отмечает прочтение" "Веб-интерфейс" {
            tags "Interaction"
        }
        timeline -> sync "Запрашивает страницы и отправляет ACK" "Вызов функции" {
            tags "Sync"
        }
        timeline -> localState "Читает сообщения и индикаторы" "Вызов функции" {
            tags "Sync"
        }
        sync -> localState "Применяет ответы и события" "Вызов функции" {
            tags "Sync"
        }
        sync -> edge "Вызывает API и подключается к событиям" "HTTPS / WSS" {
            tags "Sync"
        }
        sync -> objects "Передаёт и получает файлы по временным ссылкам" "HTTPS PUT / GET" {
            tags "Data"
        }
        edge -> authApi "Направляет запрос клиента" "ConnectRPC" {
            tags "Sync"
        }
        edge -> chatApi "Направляет запрос клиента" "ConnectRPC" {
            tags "Sync"
        }
        edge -> mediaApi "Направляет запрос клиента" "ConnectRPC" {
            tags "Sync"
        }
        edge -> presenceApi "Направляет запрос клиента" "ConnectRPC" {
            tags "Sync"
        }
        edge -> ws "Направляет запрос клиента" "WebSocket" {
            tags "Sync"
        }
        chat -> authApi "Проверяет действующий сеанс" "ConnectRPC" {
            tags "Sync"
        }
        media -> authApi "Проверяет действующий сеанс" "ConnectRPC" {
            tags "Sync"
        }
        presence -> authApi "Проверяет действующий сеанс" "ConnectRPC" {
            tags "Sync"
        }
        gateway -> authApi "Проверяет действующий сеанс" "ConnectRPC" {
            tags "Sync"
        }
        media -> chatApi "Запрашивает доступные чаты и проверяет доступ" "ConnectRPC" {
            tags "Sync"
        }
        presence -> chatApi "Запрашивает доступные чаты и проверяет доступ" "ConnectRPC" {
            tags "Sync"
        }
        gateway -> chatApi "Запрашивает доступные чаты и проверяет доступ" "ConnectRPC" {
            tags "Sync"
        }
        gateway -> presenceApi "Передаёт подключение, heartbeat и отключение" "ConnectRPC" {
            tags "Sync"
        }
        chatApi -> auth "Проверяет сеанс" "ConnectRPC" {
            tags "Sync"
        }
        mediaApi -> auth "Проверяет сеанс" "ConnectRPC" {
            tags "Sync"
        }
        presenceApi -> auth "Проверяет сеанс" "ConnectRPC" {
            tags "Sync"
        }
        authApi -> passwords "Проверяет правила пароля" "Вызов функции" {
            tags "Sync"
        }
        authApi -> credentials "Проверяет или хеширует пароль" "Вызов функции" {
            tags "Sync"
        }
        authApi -> sessions "Создаёт, обновляет или завершает сеанс" "Вызов функции" {
            tags "Sync"
        }
        authApi -> accounts "Изменяет профиль и ищет пользователей" "Вызов функции" {
            tags "Sync"
        }
        credentials -> authRepo "Получает и сохраняет хеш пароля" "Вызов функции" {
            tags "Sync"
        }
        sessions -> authRepo "Читает и сохраняет сеансы" "Вызов функции" {
            tags "Sync"
        }
        accounts -> authRepo "Читает и сохраняет профиль" "Вызов функции" {
            tags "Sync"
        }
        accounts -> db "Проверяет готовность и владельца аватара" "SQL" {
            tags "Data"
        }
        authRepo -> db "Выполняет запросы аккаунтов и сеансов" "SQL" {
            tags "Data"
        }
        authApi -> bus "Публикует событие после успешного сохранения" "NATS publish" {
            tags "Async"
        }
        chatApi -> access "Проверяет доступ к операции" "Вызов функции" {
            tags "Sync"
        }
        chatApi -> groups "Передаёт команды состава и ролей" "Вызов функции" {
            tags "Sync"
        }
        chatApi -> messages "Передаёт команды сообщения" "Вызов функции" {
            tags "Sync"
        }
        chatApi -> receipts "Передаёт подтверждения" "Вызов функции" {
            tags "Sync"
        }
        chatApi -> queries "Передаёт параметры выборки" "Вызов функции" {
            tags "Sync"
        }
        groups -> access "Проверяет роль участника" "Вызов функции" {
            tags "Sync"
        }
        groups -> chatRepo "Изменяет состав и владельца" "Вызов функции" {
            tags "Sync"
        }
        messages -> access "Проверяет членство и авторство" "Вызов функции" {
            tags "Sync"
        }
        messages -> chatRepo "Читает и сохраняет версии сообщения" "Вызов функции" {
            tags "Sync"
        }
        receipts -> access "Проверяет получателя" "Вызов функции" {
            tags "Sync"
        }
        receipts -> chatRepo "Обновляет подтверждения получателей" "Вызов функции" {
            tags "Sync"
        }
        queries -> access "Проверяет видимость диалога" "Вызов функции" {
            tags "Sync"
        }
        queries -> chatRepo "Выбирает страницы и связанные данные" "Вызов функции" {
            tags "Sync"
        }
        access -> chatRepo "Читает членство, роли и авторство" "Вызов функции" {
            tags "Sync"
        }
        chatRepo -> db "Выполняет выборки и изменения" "SQL" {
            tags "Data"
        }
        chatApi -> bus "Публикует событие после сохранения результата" "NATS publish" {
            tags "Async"
        }
        mediaApi -> assets "Проверяет доступ или выполняет операцию" "Вызов функции" {
            tags "Sync"
        }
        mediaApi -> fileAccess "Проверяет доступ или выполняет операцию" "Вызов функции" {
            tags "Sync"
        }
        assets -> filePolicy "Проверяет загруженный файл" "Вызов функции" {
            tags "Sync"
        }
        assets -> objectAdapter "Подготавливает ссылку и проверяет объект" "Вызов функции" {
            tags "Sync"
        }
        assets -> mediaRepo "Изменяет состояние загрузки" "Вызов функции" {
            tags "Sync"
        }
        fileAccess -> mediaRepo "Читает владельца и связь файла" "Вызов функции" {
            tags "Sync"
        }
        fileAccess -> chat "Проверяет актуальный доступ к чату и наличие ссылки" "ConnectRPC" {
            tags "Sync"
        }
        mediaApi -> objectAdapter "Создаёт временную ссылку после проверки доступа" "Вызов функции" {
            tags "Sync"
        }
        objectAdapter -> objects "Подписывает запросы, проверяет и удаляет объекты" "S3 API / HTTPS" {
            tags "Data"
        }
        mediaRepo -> db "Читает и изменяет метаданные" "SQL" {
            tags "Data"
        }
        presenceApi -> leases "Обновляет или читает активные подключения" "Вызов функции" {
            tags "Sync"
        }
        presenceApi -> visibility "Определяет доступные статусы" "Вызов функции" {
            tags "Sync"
        }
        leases -> redis "Сохраняет сроки и удаляет истёкшие подключения" "RESP" {
            tags "Data"
        }
        leases -> presenceEvents "Сообщает об изменении статуса" "Вызов функции" {
            tags "Sync"
        }
        visibility -> chat "Получает общие чаты и собеседников" "ConnectRPC" {
            tags "Sync"
        }
        presenceEvents -> bus "Публикует онлайн-статус" "NATS publish" {
            tags "Async"
        }
        ws -> sessionGuard "Передаёт данные IDENTIFY" "Вызов функции" {
            tags "Sync"
        }
        ws -> subscriptions "Регистрирует соединение в чатах" "Вызов функции" {
            tags "Sync"
        }
        ws -> recovery "Запрашивает пакет READY" "Вызов функции" {
            tags "Sync"
        }
        ws -> presence "Продлевает срок подключения" "ConnectRPC" {
            tags "Sync"
        }
        sessionGuard -> auth "Проверяет сеанс" "ConnectRPC" {
            tags "Sync"
        }
        subscriptions -> chat "Проверяет членство адресатов" "ConnectRPC" {
            tags "Sync"
        }
        recovery -> chat "Читает подписки и read_states" "ConnectRPC" {
            tags "Sync"
        }
        dispatch -> bus "Подписывается на subjects доменных событий" "NATS subscribe" {
            tags "Async"
        }
        dispatch -> subscriptions "Проверяет текущую аудиторию" "Вызов функции" {
            tags "Sync"
        }
        dispatch -> sessionGuard "Проверяет действующий сеанс" "Вызов функции" {
            tags "Sync"
        }
        dispatch -> ws "Передаёт кадры адресатам" "Вызов функции" {
            tags "Sync"
        }
    }
    views {
        systemContext system C1-Context "Контекст системы" {
            title "Контекст"
            include *
            autoLayout lr
            default
        }
        container system C2-Containers "Контейнеры системы" {
            title "Контейнеры"
            include *
            autoLayout tb 300 200
        }
        component web C3-web "Веб-клиент" {
            title "Веб-клиент"
            include *?
            autoLayout lr 300 160
        }
        component auth C3-auth "Сервис аккаунтов" {
            title "Сервис аккаунтов"
            include *?
            autoLayout lr 300 160
        }
        component chat C3-chat "Сервис чатов" {
            title "Сервис чатов"
            include *?
            autoLayout lr 300 160
        }
        component media C3-media "Сервис файлов" {
            title "Сервис файлов"
            include *?
            autoLayout lr 300 160
        }
        component presence C3-presence "Сервис присутствия" {
            title "Сервис присутствия"
            include *?
            autoLayout lr 300 160
        }
        component gateway C3-gateway "Шлюз событий" {
            title "Шлюз событий"
            include *?
            autoLayout lr 300 160
        }

        styles {
            element "Element" {
                color #ffffff
                stroke #334155
                fontSize 22
                metadata true
                description true
            }
            element "Person" {
                background #15324f
                shape Person
            }
            element "Software System" {
                background #174a7e
            }
            element "Container" {
                background #2563a6
            }
            element "Component" {
                background #dbeafe
                color #172b4d
                shape Component
            }
            element "Browser" {
                background #2563a6
                shape WebBrowser
            }
            element "Edge" {
                background #475569
            }
            element "Database" {
                background #166654
                shape Cylinder
            }
            element "Cache" {
                background #3c7564
                shape Cylinder
            }
            element "ObjectStore" {
                background #166654
                shape Folder
            }
            element "Queue" {
                background #9a551b
                shape Pipe
            }
            relationship "Relationship" {
                color #475569
                fontSize 18
                width 240
                routing Direct
                thickness 2
            }
            relationship "Async" {
                style dashed
                color #9a551b
            }
            relationship "Push" {
                style dashed
                color #2563a6
            }
            relationship "Data" {
                color #166654
            }
        }
        properties {
            "structurizr.sort" "key"
            "structurizr.title" "false"
            "structurizr.description" "false"
            "structurizr.metadata" "false"
            "plantuml.title" "false"
            "c4plantuml.legend" "false"
        }
    }
    configuration {
        scope softwaresystem
    }
}
