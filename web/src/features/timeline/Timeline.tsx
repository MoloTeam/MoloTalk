import { useAppState } from "../../app/AppProvider";

export function Timeline({ chatId }: { chatId: string | null }) {
  const { lastEventType, connection } = useAppState();

  return (
    <section className="timeline" aria-label="История и статусы">
      <h3>История</h3>
      <p className="hint">Диалог: {chatId ?? "не выбран"}</p>
      <p className="hint">Соединение: {connection}</p>
      <p className="hint">Последнее событие: {lastEventType ?? "нет"}</p>
      <p className="hint">История и отметки появятся в карточке Chat.</p>
    </section>
  );
}
