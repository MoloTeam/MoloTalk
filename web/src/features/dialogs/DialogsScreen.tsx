import { Link } from "react-router-dom";

import { useAppActions, useAppState } from "../../app/AppProvider";

export function DialogsScreen() {
  const { chats, selectedChatId } = useAppState();
  const actions = useAppActions();

  return (
    <section className="dialogs" aria-label="Диалоги">
      <h2>Диалоги</h2>
      <ul>
        {chats.map((chat) => (
          <li key={chat.id} className={chat.id === selectedChatId ? "selected" : undefined}>
            <button type="button" onClick={() => actions.selectChat(chat.id)}>
              {chat.title}
              <span className="badge">{chat.kind}</span>
            </button>
            <Link to={`/chats/${chat.id}`}>открыть</Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
