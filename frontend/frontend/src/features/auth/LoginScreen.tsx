import { FormEvent, useState } from 'react';
import { HardDrive, KeyRound, Loader2, Lock, Music2, ShieldCheck, ShieldOff, Sparkles } from 'lucide-react';
import { api, MusicAPIError } from '@/api/client';
import { registrationError } from './registration';

function errorMessage(error: unknown, fallback: string) {
  if (error instanceof MusicAPIError) {
    if (error.code === 'setup_required') return 'Сначала создай аккаунт владельца.';
    if (error.code === 'setup_already_completed') return 'Аккаунт владельца уже создан. Вернись ко входу.';
    if (error.code === 'invalid_totp_secret') return 'TOTP secret должен быть валидным base32-кодом из приложения-аутентификатора.';
    if (error.code === 'http_401') return 'Неверный логин, пароль или код. Проверь и попробуй ещё раз.';
    if (error.code === 'http_403') return 'Сначала нужно создать аккаунт владельца.';
    if (error.code === 'timeout') return 'Сервер не ответил вовремя. Попробуй ещё раз.';
    return error.message;
  }
  return fallback;
}

type LoginScreenProps = {
  onAuthenticated: () => void;
  onContinueLocal: () => void;
  /** Backend ответил 404 на /v1/auth/* — auth API ещё не развёрнут. */
  authUnavailable?: boolean;
  /** Первый запуск: owner ещё не создан, нужна регистрация. */
  setupRequired?: boolean;
  /** Backend ответил, но вход по паролю выключен. */
  loginDisabled?: boolean;
};

export function LoginScreen({ onAuthenticated, onContinueLocal, authUnavailable = false, setupRequired = false, loginDisabled = false }: LoginScreenProps) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [totpSecret, setTotpSecret] = useState('');
  const [showTOTPSetup, setShowTOTPSetup] = useState(false);
  const [code, setCode] = useState('');
  const [step, setStep] = useState<'register' | 'password' | 'totp'>(setupRequired ? 'register' : 'password');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [unavailable, setUnavailable] = useState(authUnavailable);

  const isRegister = setupRequired || step === 'register';
  const formDisabled = unavailable || (!isRegister && loginDisabled);

  const submitRegister = async (event: FormEvent) => {
    event.preventDefault();
    if (pending || unavailable) return;
    setError(null);
    const validation = registrationError(username, password, confirmPassword);
    if (validation) { setError(validation); return; }
    const nextUsername = username.trim();
    setPending(true);
    try {
      const result = await api.register(nextUsername, password, showTOTPSetup ? totpSecret.trim() : '');
      if (result.authenticated) onAuthenticated();
    } catch (err) {
      if (err instanceof MusicAPIError && err.code === 'http_404') { setUnavailable(true); return; }
      setError(errorMessage(err, 'Не получилось создать аккаунт владельца.'));
    } finally {
      setPending(false);
    }
  };

  const submitPassword = async (event: FormEvent) => {
    event.preventDefault();
    if (pending || formDisabled) return;
    setPending(true);
    setError(null);
    try {
      const result = await api.login(username.trim(), password);
      if (result.totp_required) {
        setStep('totp');
        setPassword('');
        return;
      }
      if (result.authenticated) onAuthenticated();
    } catch (err) {
      if (err instanceof MusicAPIError && err.code === 'http_404') { setUnavailable(true); return; }
      setError(errorMessage(err, 'Не получилось войти. Попробуй ещё раз.'));
    } finally {
      setPending(false);
    }
  };

  const submitCode = async (event: FormEvent) => {
    event.preventDefault();
    if (pending || formDisabled) return;
    setPending(true);
    setError(null);
    try {
      const result = await api.verify(code.trim());
      if (result.authenticated) onAuthenticated();
    } catch (err) {
      if (err instanceof MusicAPIError && err.code === 'http_404') { setUnavailable(true); return; }
      setError(errorMessage(err, 'Код не подошёл. Попробуй ещё раз.'));
    } finally {
      setPending(false);
    }
  };

  return <main className="grid min-h-screen place-items-center px-4 py-10">
    <div className="w-full max-w-sm">
      <div className="mb-8 grid place-items-center gap-3 text-center">
        <span className="grid h-16 w-16 place-items-center rounded-3xl bg-lime-300 text-black shadow-lg shadow-lime-300/20"><Music2 size={30} /></span>
        <div>
          <h1 className="m-0 text-2xl font-semibold tracking-tight">Music Orchestrator</h1>
          <p className="m-0 mt-1 text-sm text-[#9aa0ad]">{isRegister ? 'Первый запуск — создай аккаунт владельца' : step === 'password' ? 'Войди, чтобы открыть свою музыку' : 'Остался один шаг'}</p>
        </div>
      </div>

      {unavailable && <div className="view-enter mb-3 flex items-start gap-3 rounded-3xl border border-amber-300/25 bg-amber-300/[0.07] px-4 py-3 text-sm text-amber-100" role="status">
        <ShieldOff size={18} className="mt-0.5 shrink-0" />
        <span>Сервер авторизации недоступен. Можно продолжить локально — без аккаунта и синхронизации.</span>
      </div>}
      {!unavailable && !isRegister && loginDisabled && <div className="view-enter mb-3 flex items-start gap-3 rounded-3xl border border-amber-300/25 bg-amber-300/[0.07] px-4 py-3 text-sm text-amber-100" role="status">
        <ShieldOff size={18} className="mt-0.5 shrink-0" />
        <span>Вход пока недоступен: аккаунт владельца ещё не создан.</span>
      </div>}

      {isRegister ? <form onSubmit={submitRegister} className="view-enter grid gap-3 rounded-3xl border border-white/10 bg-white/[0.035] p-6">
        <div className="flex items-start gap-3 rounded-2xl border border-lime-300/20 bg-lime-300/[0.06] px-3 py-2.5 text-sm text-lime-100">
          <Sparkles size={18} className="mt-0.5 shrink-0" />
          <span>Владелец управляет доступом к медиатеке. Аккаунт хранится на этом сервере.</span>
        </div>
        <label className="grid gap-2 text-sm text-[#a6abb7]">Логин
          <span className="flex h-12 items-center gap-3 rounded-2xl border border-white/15 bg-[#101217] px-4 transition focus-within:border-lime-300/60">
            <KeyRound size={17} className="shrink-0 text-[#8c919e]" />
            <input autoFocus autoComplete="username" disabled={unavailable} value={username} onChange={(e) => setUsername(e.target.value)} className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-[#626875] disabled:opacity-50" placeholder="Придумай логин" />
          </span>
        </label>
        <label className="grid gap-2 text-sm text-[#a6abb7]">Пароль
          <span className="flex h-12 items-center gap-3 rounded-2xl border border-white/15 bg-[#101217] px-4 transition focus-within:border-lime-300/60">
            <Lock size={17} className="shrink-0 text-[#8c919e]" />
            <input type="password" autoComplete="new-password" disabled={unavailable} value={password} onChange={(e) => setPassword(e.target.value)} className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-[#626875] disabled:opacity-50" placeholder="минимум 10 символов" />
          </span>
        </label>
        <label className="grid gap-2 text-sm text-[#a6abb7]">Повтори пароль
          <span className="flex h-12 items-center gap-3 rounded-2xl border border-white/15 bg-[#101217] px-4 transition focus-within:border-lime-300/60">
            <Lock size={17} className="shrink-0 text-[#8c919e]" />
            <input type="password" autoComplete="new-password" disabled={unavailable} value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-[#626875] disabled:opacity-50" placeholder="ещё раз" />
          </span>
        </label>
        <button type="button" onClick={() => setShowTOTPSetup((value) => !value)} className="rounded-2xl border border-white/10 px-3 py-2 text-left text-sm text-[#c6cad3] transition hover:bg-white/8 hover:text-white">
          {showTOTPSetup ? 'Скрыть двухфакторную защиту' : 'Включить двухфакторную защиту'}
          <span className="mt-1 block text-xs text-[#747b89]">Необязательно. Вставь секретный ключ из приложения-аутентификатора — тогда после пароля будет запрашиваться шестизначный код.</span>
        </button>
        {showTOTPSetup && <label className="grid gap-2 text-sm text-[#a6abb7]">TOTP secret
          <input value={totpSecret} onChange={(e) => setTotpSecret(e.target.value.toUpperCase())} className="h-12 rounded-2xl border border-white/15 bg-[#101217] px-4 font-mono text-sm outline-none transition focus:border-lime-300/60 placeholder:text-[#626875]" placeholder="JBSWY3DPEHPK3PXP" />
        </label>}
        <ul className="m-0 grid gap-1 px-1 text-xs" aria-live="polite">
          {[
            { ok: username.trim().length > 0, label: 'Логин заполнен' },
            { ok: password.length >= 10, label: 'Пароль минимум 10 символов' },
            { ok: confirmPassword.length > 0 && password === confirmPassword, label: 'Пароли совпадают' },
          ].map((hint) => <li key={hint.label} className={hint.ok ? 'text-lime-300' : 'text-[#747b89]'}>
            {hint.ok ? '✓' : '•'} {hint.label}
          </li>)}
        </ul>
        {error && <p role="alert" className="m-0 rounded-2xl border border-red-300/30 bg-red-300/[0.07] px-3 py-2 text-sm text-red-100">{error}</p>}
        <button type="submit" disabled={unavailable || pending} className="mt-1 inline-flex h-12 items-center justify-center gap-2 rounded-2xl bg-lime-300 font-medium text-black transition hover:bg-lime-200 active:scale-[0.99] disabled:opacity-50">
          {pending && <Loader2 className="animate-spin" size={17} />} Создать аккаунт
        </button>
        {unavailable && <button type="button" onClick={onContinueLocal} className="inline-flex h-11 items-center justify-center gap-2 rounded-2xl border border-white/10 text-sm text-[#c6cad3] transition hover:bg-white/8 hover:text-white active:scale-[0.99]"><HardDrive size={16} /> Продолжить локально</button>}
      </form> : step === 'password' ? <form onSubmit={submitPassword} className="view-enter grid gap-3 rounded-3xl border border-white/10 bg-white/[0.035] p-6">
        <label className="grid gap-2 text-sm text-[#a6abb7]">Логин
          <span className="flex h-12 items-center gap-3 rounded-2xl border border-white/15 bg-[#101217] px-4 transition focus-within:border-lime-300/60">
            <KeyRound size={17} className="shrink-0 text-[#8c919e]" />
            <input autoFocus autoComplete="username" disabled={formDisabled} value={username} onChange={(e) => setUsername(e.target.value)} className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-[#626875] disabled:opacity-50" placeholder="admin" />
          </span>
        </label>
        <label className="grid gap-2 text-sm text-[#a6abb7]">Пароль
          <span className="flex h-12 items-center gap-3 rounded-2xl border border-white/15 bg-[#101217] px-4 transition focus-within:border-lime-300/60">
            <Lock size={17} className="shrink-0 text-[#8c919e]" />
            <input type="password" autoComplete="current-password" disabled={formDisabled} value={password} onChange={(e) => setPassword(e.target.value)} className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-[#626875] disabled:opacity-50" placeholder="••••••••" />
          </span>
        </label>
        {error && <p role="alert" className="m-0 rounded-2xl border border-red-300/30 bg-red-300/[0.07] px-3 py-2 text-sm text-red-100">{error}</p>}
        <button type="submit" disabled={formDisabled || pending || !username.trim() || !password} className="mt-1 inline-flex h-12 items-center justify-center gap-2 rounded-2xl bg-lime-300 font-medium text-black transition hover:bg-lime-200 active:scale-[0.99] disabled:opacity-50">
          {pending && <Loader2 className="animate-spin" size={17} />} Войти
        </button>
        <button type="button" onClick={onContinueLocal} className="inline-flex h-11 items-center justify-center gap-2 rounded-2xl border border-white/10 text-sm text-[#c6cad3] transition hover:bg-white/8 hover:text-white active:scale-[0.99]">
          <HardDrive size={16} /> Продолжить локально
        </button>
      </form> : <form onSubmit={submitCode} className="view-enter grid gap-3 rounded-3xl border border-white/10 bg-white/[0.035] p-6">
        <div className="flex items-start gap-3 rounded-2xl border border-lime-300/20 bg-lime-300/[0.06] px-3 py-2.5 text-sm text-lime-100">
          <ShieldCheck size={18} className="mt-0.5 shrink-0" />
          <span>Открой приложение-аутентификатор и введи шестизначный код.</span>
        </div>
        <label className="grid gap-2 text-sm text-[#a6abb7]">Код из приложения
          <input autoFocus inputMode="numeric" pattern="[0-9]*" maxLength={6} autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))} className="h-13 rounded-2xl border border-white/15 bg-[#101217] px-4 text-center font-mono text-2xl tracking-[0.5em] outline-none transition focus:border-lime-300/60 placeholder:text-[#626875]" placeholder="000000" />
        </label>
        {error && <p role="alert" className="m-0 rounded-2xl border border-red-300/30 bg-red-300/[0.07] px-3 py-2 text-sm text-red-100">{error}</p>}
        <button type="submit" disabled={pending || code.length !== 6} className="mt-1 inline-flex h-12 items-center justify-center gap-2 rounded-2xl bg-lime-300 font-medium text-black transition hover:bg-lime-200 active:scale-[0.99] disabled:opacity-50">
          {pending && <Loader2 className="animate-spin" size={17} />} Подтвердить
        </button>
        <button type="button" onClick={() => { setStep('password'); setCode(''); setError(null); }} className="h-11 rounded-2xl text-sm text-[#9aa0ad] transition hover:bg-white/8 hover:text-white">← Назад ко входу</button>
      </form>}

      <p className="m-0 mt-4 text-center text-xs text-[#626875]">{unavailable ? 'Локальный режим — только для разработки, без production-авторизации.' : 'Музыка и аккаунт хранятся на твоём сервере.'}</p>
    </div>
  </main>;
}
