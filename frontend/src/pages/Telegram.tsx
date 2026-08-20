import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Send,
  KeyRound,
  LogOut,
  Radio,
  FolderOpen,
  Download,
  Terminal,
  RefreshCw,
  Play,
  Square,
  Loader2,
  AlertTriangle,
  X,
} from 'lucide-react';
import { useAppStore } from '../store/useAppStore';

export const TelegramPage: React.FC = () => {
  const {
    tgState,
    tgChannels,
    tgSelected,
    tgKeywords,
    tgLogs,
    tgDownloads,
    tgCodeSent,
    tgBusy,
    tgError,
    tgInit,
    tgSendCode,
    tgConfirmCode,
    tgLogout,
    tgFetchChannels,
    tgToggleChannel,
    tgSetKeywords,
    tgStartMonitor,
    tgStopMonitor,
    tgOpenTempDir,
    tgClearLogs,
    tgClearError,
  } = useAppStore();

  const [apiId, setApiId] = useState('');
  const [apiHash, setApiHash] = useState('');
  const [phone, setPhone] = useState('');
  const [otp, setOtp] = useState('');
  const [password, setPassword] = useState('');
  const [filter, setFilter] = useState('');

  const logRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    tgInit();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight, behavior: 'smooth' });
  }, [tgLogs]);

  const filteredChannels = useMemo(
    () =>
      tgChannels.filter((c) =>
        c.title.toLowerCase().includes(filter.toLowerCase())
      ),
    [tgChannels, filter]
  );

  const lastLogs = useMemo(() => tgLogs.slice(-3), [tgLogs]);

  const handleSendCode = async (e: React.FormEvent) => {
    e.preventDefault();
    const id = parseInt(apiId, 10);
    if (!id || !apiHash.trim() || !phone.trim()) return;
    tgClearError();
    await tgSendCode(id, apiHash.trim(), phone.trim());
  };

  const handleConfirm = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!otp.trim()) return;
    tgClearError();
    const ok = await tgConfirmCode(otp.trim(), password);
    if (ok) {
      setOtp('');
      setPassword('');
    }
  };

  const statusBadge = (on: boolean, onText: string, offText: string) => (
    <span
      className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[10px] font-bold uppercase tracking-wider border ${
        on
          ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30'
          : 'bg-[#14141a] text-slate-500 border-[#1f1f26]'
      }`}
    >
      <span className={`w-1.5 h-1.5 rounded-full ${on ? 'bg-emerald-400 animate-pulse' : 'bg-slate-600'}`} />
      {on ? onText : offText}
    </span>
  );

  return (
    <div className="space-y-5 max-w-6xl mx-auto">
      <div className="flex items-start justify-between">
        <div>
          <h2 className="text-slate-100 text-lg font-bold uppercase tracking-wider">
            Telegram Auto
          </h2>
          <p className="text-xs text-slate-400 mt-0.5">
            Monitor channels for keyworded archives, auto-download, extract and check cookies.
          </p>
        </div>
        <div className="flex items-center gap-2">
          {statusBadge(tgState.configured, 'Configured', 'Not configured')}
          {statusBadge(tgState.loggedIn, 'Logged in', 'Logged out')}
          {statusBadge(tgState.monitoring, 'Monitoring', 'Idle')}
        </div>
      </div>

      {tgError && (
        <div className="flex items-center gap-2.5 bg-red-500/10 border border-red-500/40 rounded-lg px-4 py-2.5 text-xs text-red-300">
          <AlertTriangle className="w-4 h-4 shrink-0 text-red-400" />
          <span className="flex-1 break-all">{tgError}</span>
          <button
            onClick={tgClearError}
            className="text-red-400/70 hover:text-red-300 cursor-pointer transition-colors"
            aria-label="Dismiss error"
          >
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
        {/* Credentials & Login */}
        <div className="bg-[#0e0e12] border border-[#1f1f26] rounded-xl p-5 space-y-4">
          <div className="flex items-center gap-2 text-xs font-bold text-slate-200 uppercase tracking-wider border-b border-[#16161c] pb-2">
            <KeyRound className="w-4 h-4 text-cyan-500" />
            Credentials & Login
          </div>

          <form onSubmit={handleSendCode} className="space-y-3 text-xs">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label className="block text-slate-400 font-medium mb-1">API ID</label>
                <input
                  type="text"
                  value={apiId}
                  onChange={(e) => setApiId(e.target.value)}
                  placeholder="1234567"
                  disabled={tgState.loggedIn}
                  className="w-full bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200 disabled:opacity-50"
                />
              </div>
              <div>
                <label className="block text-slate-400 font-medium mb-1">API Hash</label>
                <input
                  type="text"
                  value={apiHash}
                  onChange={(e) => setApiHash(e.target.value)}
                  placeholder="0123456789abcdef0123456789abcdef"
                  disabled={tgState.loggedIn}
                  className="w-full bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200 disabled:opacity-50"
                />
              </div>
            </div>

            <div>
              <label className="block text-slate-400 font-medium mb-1">Phone Number</label>
              <input
                type="text"
                value={phone}
                onChange={(e) => setPhone(e.target.value)}
                placeholder="+12345678901"
                disabled={tgState.loggedIn}
                className="w-full bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200 disabled:opacity-50"
              />
            </div>

            {!tgState.loggedIn && (
              <button
                type="submit"
                disabled={tgBusy || !apiId.trim() || !apiHash.trim() || !phone.trim()}
                className="inline-flex items-center gap-2 px-4 py-2 bg-cyan-500 hover:bg-cyan-400 disabled:opacity-50 text-slate-950 text-xs font-bold rounded-md shadow-lg shadow-cyan-500/10 active:scale-95 transition-all duration-150 cursor-pointer"
              >
                {tgBusy ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Send className="w-3.5 h-3.5" />}
                Send verification code
              </button>
            )}
          </form>

          {tgCodeSent && !tgState.loggedIn && (
            <form onSubmit={handleConfirm} className="space-y-3 text-xs border-t border-[#16161c] pt-3">
              <div>
                <label className="block text-slate-400 font-medium mb-1">Verification Code (OTP)</label>
                <input
                  type="text"
                  value={otp}
                  onChange={(e) => setOtp(e.target.value)}
                  placeholder="12345"
                  autoFocus
                  className="w-full bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200"
                />
              </div>
              <div>
                <label className="block text-slate-400 font-medium mb-1">
                  2FA Password <span className="text-slate-600">(only if enabled)</span>
                </label>
                <input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="Optional two-factor password"
                  className="w-full bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200"
                />
              </div>
              <button
                type="submit"
                disabled={tgBusy || !otp.trim()}
                className="inline-flex items-center gap-2 px-4 py-2 bg-emerald-500 hover:bg-emerald-400 disabled:opacity-50 text-slate-950 text-xs font-bold rounded-md shadow-lg shadow-emerald-500/10 active:scale-95 transition-all duration-150 cursor-pointer"
              >
                {tgBusy ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <KeyRound className="w-3.5 h-3.5" />}
                Sign in
              </button>
            </form>
          )}

          {tgState.loggedIn && (
            <div className="border-t border-[#16161c] pt-3 flex items-center justify-between">
              <span className="text-xs text-slate-400">
                Session active
                {tgState.user ? (
                  <>
                    {' '}
                    as <span className="text-cyan-400 font-semibold">{tgState.user}</span>
                  </>
                ) : null}
              </span>
              <button
                onClick={tgLogout}
                disabled={tgBusy || tgState.monitoring}
                className="inline-flex items-center gap-2 px-3 py-1.5 bg-[#14141a] hover:bg-[#1f1f26] disabled:opacity-50 border border-[#1f1f26] text-slate-200 rounded-md text-xs font-medium transition-colors cursor-pointer"
              >
                <LogOut className="w-3.5 h-3.5" />
                Logout
              </button>
            </div>
          )}

          <div className="border-t border-[#16161c] pt-3">
            <div className="flex items-center gap-2 mb-1.5">
              <span className="relative flex h-1.5 w-1.5">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-cyan-400 opacity-75" />
                <span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-cyan-400" />
              </span>
              <span className="text-[10px] font-bold text-cyan-400 uppercase tracking-wider">Live log</span>
            </div>
            <div className="font-mono text-[10px] text-slate-500 space-y-0.5 bg-[#040406] border border-[#16161c] rounded-md px-2.5 py-2">
              {lastLogs.length === 0 ? (
                <span className="text-slate-600">Waiting for activity...</span>
              ) : (
                lastLogs.map((l) => (
                  <div key={l.id} className="flex gap-2 break-all">
                    <span className="shrink-0">{l.timestamp}</span>
                    <span
                      className={`shrink-0 font-bold ${
                        l.level === 'error'
                          ? 'text-red-400'
                          : l.level === 'warn'
                            ? 'text-amber-400'
                            : l.level === 'success'
                              ? 'text-emerald-400'
                              : 'text-cyan-400'
                      }`}
                    >
                      {l.level.toUpperCase().padEnd(7, ' ')}
                    </span>
                    <span className="text-slate-400">{l.message}</span>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>

        {/* Channels */}
        <div className="bg-[#0e0e12] border border-[#1f1f26] rounded-xl p-5 space-y-4">
          <div className="flex items-center justify-between border-b border-[#16161c] pb-2">
            <div className="flex items-center gap-2 text-xs font-bold text-slate-200 uppercase tracking-wider">
              <Radio className="w-4 h-4 text-cyan-500" />
              Channels
            </div>
            <button
              onClick={tgFetchChannels}
              disabled={tgBusy || !tgState.loggedIn}
              className="inline-flex items-center gap-2 px-3 py-1.5 bg-[#14141a] hover:bg-[#1f1f26] disabled:opacity-50 border border-[#1f1f26] text-slate-200 rounded-md text-xs font-medium transition-colors cursor-pointer"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${tgBusy ? 'animate-spin' : ''}`} />
              Load channels
            </button>
          </div>

          {tgState.loggedIn && (
            <>
              <input
                type="text"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                placeholder="Filter channels..."
                className="w-full bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-xs text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200"
              />
              <div className="max-h-48 overflow-y-auto space-y-1 pr-1">
                {filteredChannels.length === 0 && (
                  <div className="text-xs text-slate-600 text-center py-6">
                    {tgChannels.length === 0
                      ? 'No channels loaded yet — click "Load channels".'
                      : 'No channels match the filter.'}
                  </div>
                )}
                {filteredChannels.map((c) => (
                  <label
                    key={c.id}
                    className="flex items-center gap-2.5 px-2 py-1.5 rounded-md hover:bg-[#14141a] cursor-pointer transition-colors"
                  >
                    <input
                      type="checkbox"
                      checked={tgSelected.includes(c.id)}
                      onChange={() => tgToggleChannel(c.id)}
                      className="rounded bg-[#040406] border-[#1f1f26] text-cyan-500 focus:ring-0"
                    />
                    <span className="flex-1 text-xs text-slate-200 truncate">{c.title}</span>
                    {c.username && (
                      <span className="text-[10px] text-slate-600">@{c.username}</span>
                    )}
                    <span
                      className={`text-[10px] font-bold uppercase px-1.5 py-0.5 rounded ${
                        c.broadcast ? 'bg-cyan-500/10 text-cyan-400' : 'bg-amber-500/10 text-amber-400'
                      }`}
                    >
                      {c.broadcast ? 'Channel' : 'Group'}
                    </span>
                  </label>
                ))}
              </div>
              <div className="text-[10px] text-slate-500">
                {tgSelected.length} selected · {tgChannels.length} loaded
              </div>
            </>
          )}
        </div>
      </div>

      {/* Monitor Controls */}
      <div className="bg-[#0e0e12] border border-[#1f1f26] rounded-xl p-5 space-y-4">
        <div className="flex items-center gap-2 text-xs font-bold text-slate-200 uppercase tracking-wider border-b border-[#16161c] pb-2">
          <Play className="w-4 h-4 text-cyan-500" />
          Monitor
        </div>

        <div>
          <label className="block text-slate-400 font-medium mb-1 text-xs">
            Keywords <span className="text-slate-600">(comma separated)</span>
          </label>
          <input
            type="text"
            value={tgKeywords}
            onChange={(e) => tgSetKeywords(e.target.value)}
            placeholder="logs, updaisy, fresh, uhq, cracked"
            className="w-full bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-xs text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200"
          />
        </div>

        <div className="flex items-center gap-3">
          {!tgState.monitoring ? (
            <button
              onClick={tgStartMonitor}
              disabled={tgBusy || !tgState.loggedIn || !tgSelected.length || !tgKeywords.trim()}
              className="inline-flex items-center gap-2 px-4 py-2 bg-cyan-500 hover:bg-cyan-400 disabled:opacity-50 text-slate-950 text-xs font-bold rounded-md shadow-lg shadow-cyan-500/10 active:scale-95 transition-all duration-150 cursor-pointer"
            >
              {tgBusy ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Play className="w-3.5 h-3.5" />}
              Start monitoring
            </button>
          ) : (
            <button
              onClick={tgStopMonitor}
              disabled={tgBusy}
              className="inline-flex items-center gap-2 px-4 py-2 bg-red-500/80 hover:bg-red-500 disabled:opacity-50 text-slate-950 text-xs font-bold rounded-md shadow-lg shadow-red-500/10 active:scale-95 transition-all duration-150 cursor-pointer"
            >
              <Square className="w-3.5 h-3.5" />
              Stop monitoring
            </button>
          )}
          <button
            onClick={tgOpenTempDir}
            className="inline-flex items-center gap-2 px-3 py-2 bg-[#14141a] hover:bg-[#1f1f26] border border-[#1f1f26] text-slate-200 rounded-md text-xs font-medium transition-colors cursor-pointer"
          >
            <FolderOpen className="w-3.5 h-3.5" />
            Open Telegram_temp
          </button>
        </div>
      </div>

      {/* Activity Log */}
      <div className="bg-[#0e0e12] border border-[#1f1f26] rounded-xl p-5 space-y-3">
        <div className="flex items-center justify-between border-b border-[#16161c] pb-2">
          <div className="flex items-center gap-2 text-xs font-bold text-slate-200 uppercase tracking-wider">
            <Terminal className="w-4 h-4 text-cyan-500" />
            Live Activity
            <span className="flex items-center gap-1 text-[9px] text-cyan-400 font-bold">
              <span className="relative flex h-1.5 w-1.5">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-cyan-400 opacity-75" />
                <span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-cyan-400" />
              </span>
              LIVE
            </span>
          </div>
          <button
            onClick={tgClearLogs}
            className="text-[10px] text-slate-500 hover:text-slate-300 uppercase tracking-wider cursor-pointer transition-colors"
          >
            Clear
          </button>
        </div>
        <div ref={logRef} className="h-56 overflow-y-auto bg-[#040406] border border-[#16161c] rounded-md p-3 font-mono text-[11px] space-y-1">
          {tgLogs.length === 0 && <div className="text-slate-600">—</div>}
          {tgLogs.map((l) => (
            <div key={l.id} className="flex gap-2">
              <span className="text-slate-600 shrink-0">{l.timestamp}</span>
              <span
                className={`shrink-0 font-bold ${
                  l.level === 'error'
                    ? 'text-red-400'
                    : l.level === 'warn'
                      ? 'text-amber-400'
                      : l.level === 'success'
                        ? 'text-emerald-400'
                        : 'text-cyan-400'
                }`}
              >
                {l.level.toUpperCase().padEnd(7, ' ')}
              </span>
              <span className="text-slate-300 break-all">{l.message}</span>
            </div>
          ))}
        </div>
      </div>

      {/* Downloads */}
      <div className="bg-[#0e0e12] border border-[#1f1f26] rounded-xl p-5 space-y-3">
        <div className="flex items-center gap-2 text-xs font-bold text-slate-200 uppercase tracking-wider border-b border-[#16161c] pb-2">
          <Download className="w-4 h-4 text-cyan-500" />
          Downloads
        </div>
        {tgDownloads.length === 0 ? (
          <div className="text-xs text-slate-600 text-center py-6">
            Archives matching your keywords will appear here.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-left text-[10px] text-slate-500 uppercase tracking-wider border-b border-[#16161c]">
                  <th className="py-2 pr-3">Time</th>
                  <th className="py-2 pr-3">Channel</th>
                  <th className="py-2 pr-3">File</th>
                  <th className="py-2 pr-3">Password</th>
                  <th className="py-2 pr-3">Extracted</th>
                  <th className="py-2">Message</th>
                </tr>
              </thead>
              <tbody>
                {tgDownloads.map((d, i) => (
                  <tr key={i} className="border-b border-[#121218]">
                    <td className="py-2 pr-3 text-slate-500 whitespace-nowrap">
                      {new Date().toLocaleTimeString()}
                    </td>
                    <td className="py-2 pr-3 text-slate-300 whitespace-nowrap max-w-[140px] truncate">
                      {d.channel}
                    </td>
                    <td className="py-2 pr-3 text-slate-200 whitespace-nowrap max-w-[180px] truncate">
                      {d.fileName}
                    </td>
                    <td className="py-2 pr-3 text-amber-400/80 whitespace-nowrap max-w-[100px] truncate">
                      {d.password || '—'}
                    </td>
                    <td className="py-2 pr-3">
                      {d.extractOK ? (
                        <span className="text-emerald-400 font-bold">OK</span>
                      ) : (
                        <span className="text-red-400 font-bold">Failed</span>
                      )}
                    </td>
                    <td className="py-2 text-slate-600 max-w-[200px] truncate">{d.messageText}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
};