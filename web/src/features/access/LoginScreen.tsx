export function LoginScreen() {
  return (
    <section className="screen" aria-label="Вход">
      <h1>Вход</h1>
      <p className="hint">Каркас экрана: форма и проверки появятся в карточке Auth.</p>
      <form className="skeleton-form" onSubmit={(event) => event.preventDefault()}>
        <label>
          Идентификатор
          <input type="text" name="identifier" autoComplete="username" disabled />
        </label>
        <label>
          Пароль
          <input type="password" name="password" autoComplete="current-password" disabled />
        </label>
        <button type="submit" disabled>
          Войти
        </button>
      </form>
    </section>
  );
}

