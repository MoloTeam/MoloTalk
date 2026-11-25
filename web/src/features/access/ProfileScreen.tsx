import { useAppActions, useAppState } from "../../app/AppProvider";

export function ProfileScreen() {
  const { profile, error } = useAppState();
  const actions = useAppActions();

  return (
    <section className="screen" aria-label="Профиль">
      <h1>Профиль</h1>
      {profile ? (
        <p>
          Вы вошли как <strong>{profile.displayName}</strong> ({profile.id})
        </p>
      ) : (
        <p className="hint">Профиль ещё не загружен.</p>
      )}
      {error ? <p className="error" role="alert">{error}</p> : null}
      <button type="button" onClick={() => void actions.loadProfile()}>
        Загрузить профиль
      </button>
    </section>
  );
}
