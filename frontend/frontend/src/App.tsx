import { useState } from 'react';
import { Loader2, Music2 } from 'lucide-react';
import { useLogout, useSession } from '@/api/queries';
import { LoginScreen } from '@/features/auth/LoginScreen';
import { SearchPage } from '@/features/search/SearchPage';

const LOCAL_MODE_KEY = 'music.localMode';

function Splash() {
  return <main className="grid min-h-screen place-items-center px-4">
    <div className="grid place-items-center gap-4 text-center">
      <span className="grid h-16 w-16 place-items-center rounded-3xl bg-lime-300 text-black shadow-lg shadow-lime-300/20"><Music2 size={30} /></span>
      <p className="m-0 flex items-center gap-2 text-sm text-[#9aa0ad]"><Loader2 className="animate-spin text-lime-300" size={16} /> Проверяю сессию…</p>
    </div>
  </main>;
}

export default function App() {
  const session = useSession();
  const logout = useLogout();
  const [localMode, setLocalMode] = useState(() => typeof window !== 'undefined' && window.sessionStorage.getItem(LOCAL_MODE_KEY) === '1');

  const continueLocal = () => {
    window.sessionStorage.setItem(LOCAL_MODE_KEY, '1');
    setLocalMode(true);
  };
  const leaveLocalMode = () => {
    window.sessionStorage.removeItem(LOCAL_MODE_KEY);
    setLocalMode(false);
  };

  if (localMode) return <SearchPage localMode onLeaveLocalMode={leaveLocalMode} />;
  if (session.isLoading) return <Splash />;

  // Backend without the auth API (404/disabled) or unreachable: friendly fallback, dev can continue locally.
  if (session.isError) {
    return <LoginScreen authUnavailable onAuthenticated={() => session.refetch()} onContinueLocal={continueLocal} />;
  }

  const info = session.data;
  if (info?.authenticated) return <SearchPage onLogout={() => logout.mutate()} />;
  return <LoginScreen setupRequired={Boolean(info?.setup_required)} loginDisabled={info ? !info.login_enabled : false} onAuthenticated={() => session.refetch()} onContinueLocal={continueLocal} />;
}
