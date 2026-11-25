export function Composer({ chatId }: { chatId: string | null }) {
  return (
    <section className="composer" aria-label="Редактор">
      <h3>Редактор</h3>
      <p className="hint">Диалог: {chatId ?? "не выбран"}</p>
      <textarea placeholder="Текст сообщения" disabled rows={3} />
      <button type="button" disabled>
        Отправить
      </button>
    </section>
  );
}
