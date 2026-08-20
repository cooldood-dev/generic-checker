import React, { useEffect, useRef, useState } from 'react';
import { Play, Square, FolderOpen } from 'lucide-react';
import { MetricCard } from '../components/dashboard/MetricCard';
import { StatGroup, StatRow } from '../components/dashboard/StatGroup';
import { ProgressBar } from '../components/dashboard/ProgressBar';
import { StatusBadge } from '../components/StatusBadge';
import { useAppStore, LogEntry } from '../store/useAppStore';

function logColor(log: LogEntry): string {
  if (log.level === 'success' || log.message.includes('[UNBANNED]') || log.message.includes('[HIT]')) {
    return 'text-emerald-400';
  }
  if (log.level === 'warn' || log.message.includes('[RATE LIMITED]') || log.message.includes('[BANNED]')) {
    return 'text-amber-400';
  }
  if (
    log.level === 'error' ||
    log.message.includes('[INVALID]') ||
    log.message.includes('[ERROR]') ||
    log.message.includes('[BLOCKED]')
  ) {
    return 'text-rose-400';
  }
  if (log.message.includes('[NO MC]')) {
    return 'text-cyan-400';
  }
  return 'text-slate-400';
}

export const CheckPage: React.FC = () => {
  const { isChecking, stats, liveHits, logs, startChecking, stopChecking, openResults } = useAppStore();
  const logEndRef = useRef<HTMLDivElement>(null);
  const logBoxRef = useRef<HTMLDivElement>(null);
  const [banTab, setBanTab] = useState<'hypixel' | 'donut' | 'minemen'>('hypixel');

  const progressPercent = stats.total > 0 ? (stats.processed / stats.total) * 100 : 0;

  // Keep the viewport pinned to the newest line (bottom).
  useEffect(() => {
    const box = logBoxRef.current;
    if (box) {
      box.scrollTop = box.scrollHeight;
    } else {
      logEndRef.current?.scrollIntoView({ behavior: 'auto', block: 'end' });
    }
  }, [logs]);

  const formatTime = (totalSec: number) => {
    const mins = Math.floor(totalSec / 60);
    const secs = totalSec % 60;
    return `${mins}:${secs.toString().padStart(2, '0')}`;
  };

  const getStatusBadgeProps = (status: string, banDuration?: string) => {
    switch (status) {
      case 'unbanned':
        return { type: 'healthy' as const, label: 'UNBANNED' };
      case 'banned':
        return { type: 'error' as const, label: banDuration ? `BANNED | ${banDuration}` : 'BANNED' };
      case 'blocked':
        return { type: 'warning' as const, label: 'BLOCKED' };
      case 'no_mc':
        return { type: 'idle' as const, label: 'NO MC' };
      default:
        return { type: 'idle' as const, label: 'INVALID' };
    }
  };

  return (
    <div className="space-y-5 max-w-7xl mx-auto">
      <div className="flex flex-wrap items-center justify-between gap-4 bg-[#0e0e12]/85 border border-[#1f1f26] p-4 rounded-xl shadow-md backdrop-blur-md">
        <div className="flex items-center gap-3">
          <span
            className={`w-2.5 h-2.5 rounded-full ${
              isChecking ? 'bg-cyan-500 animate-pulse' : stats.processed > 0 ? 'bg-emerald-500' : 'bg-slate-600'
            }`}
          />
          <div>
            <div className="text-sm font-semibold text-slate-100 tracking-wide">
              {isChecking
                ? 'Running live diagnostic checking pipeline...'
                : stats.processed > 0
                ? 'Checking run complete · all done'
                : 'Ready to start checking'}
            </div>
            <div className="text-xs text-slate-400 mt-0.5 font-mono">
              Processed {stats.processed.toLocaleString()} of {stats.total.toLocaleString()} accounts
            </div>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {!isChecking ? (
            <button
              onClick={startChecking}
              className="inline-flex items-center gap-2 px-4 py-2 bg-cyan-500 hover:bg-cyan-400 text-slate-950 text-xs font-bold rounded-md shadow-lg shadow-cyan-500/10 active:scale-95 transition-all duration-150 cursor-pointer"
            >
              <Play className="w-3.5 h-3.5 fill-current" />
              Start Checking
            </button>
          ) : (
            <button
              onClick={stopChecking}
              className="inline-flex items-center gap-2 px-4 py-2 bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 text-xs font-bold rounded-md active:scale-95 transition-all duration-150 cursor-pointer"
            >
              <Square className="w-3.5 h-3.5 fill-current" />
              Stop Checking
            </button>
          )}

          <button
            onClick={() => openResults()}
            className="inline-flex items-center gap-2 px-4 py-2 bg-[#14141a] hover:bg-[#1f1f26] border border-[#1f1f26] text-slate-200 text-xs font-bold rounded-md active:scale-95 transition-all duration-150 cursor-pointer"
          >
            <FolderOpen className="w-3.5 h-3.5 text-slate-400" />
            Open Results
          </button>
        </div>
      </div>

      <ProgressBar progress={progressPercent} label="Live Pipeline Execution Progress" />

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3.5">
        <MetricCard
          label="CHECKED"
          value={stats.processed.toLocaleString()}
          subtext={`out of ${stats.total.toLocaleString()} target items`}
        />
        <MetricCard
          label="HITS (VALID MS)"
          value={stats.hits.toLocaleString()}
          subtext="passed authentication"
          highlight={true}
        />
        <MetricCard label="CPM" value={stats.cpm.toLocaleString()} subtext="realtime checks / minute" />
        <MetricCard label="ELAPSED TIME" value={formatTime(stats.elapsedSeconds)} subtext="active run time" />
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3.5">
        {/* Tabbed Ban Checks Panel */}
        <div className="bg-[#0e0e12]/85 border border-[#1f1f26] rounded-xl shadow-md backdrop-blur-md overflow-hidden">
          {/* Tab Header */}
          <div className="flex border-b border-[#16161c]">
            <button
              id="ban-tab-hypixel"
              onClick={() => setBanTab('hypixel')}
              className={`flex-1 flex items-center justify-center gap-1.5 px-3 py-2.5 text-[10px] font-bold uppercase tracking-wider transition-all duration-200 cursor-pointer ${
                banTab === 'hypixel'
                  ? 'bg-cyan-500/10 text-cyan-400 border-b-2 border-cyan-500'
                  : 'text-slate-500 hover:text-slate-300 hover:bg-[#14141a]'
              }`}
            >
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 shrink-0" />
              Hypixel
            </button>
            <button
              id="ban-tab-donut"
              onClick={() => setBanTab('donut')}
              className={`flex-1 flex items-center justify-center gap-1.5 px-3 py-2.5 text-[10px] font-bold uppercase tracking-wider transition-all duration-200 cursor-pointer ${
                banTab === 'donut'
                  ? 'bg-amber-500/10 text-amber-400 border-b-2 border-amber-500'
                  : 'text-slate-500 hover:text-slate-300 hover:bg-[#14141a]'
              }`}
            >
              <span className="w-1.5 h-1.5 rounded-full bg-amber-500 shrink-0" />
              DonutSMP
            </button>
            <button
              id="ban-tab-minemen"
              onClick={() => setBanTab('minemen')}
              className={`flex-1 flex items-center justify-center gap-1.5 px-3 py-2.5 text-[10px] font-bold uppercase tracking-wider transition-all duration-200 cursor-pointer ${
                banTab === 'minemen'
                  ? 'bg-purple-500/10 text-purple-400 border-b-2 border-purple-500'
                  : 'text-slate-500 hover:text-slate-300 hover:bg-[#14141a]'
              }`}
            >
              <span className="w-1.5 h-1.5 rounded-full bg-purple-500 shrink-0" />
              Minemen
            </button>
          </div>

          {/* Tab Content */}
          <div className="p-4 space-y-1">
            {banTab === 'hypixel' && (
              <>
                <div className="text-[9px] font-bold text-slate-500 uppercase tracking-widest mb-2">
                  mc.hypixel.net — via MC Protocol
                </div>
                <StatRow label="Unbanned Hits" value={stats.unbanned.toLocaleString()} color="success" />
                <StatRow label="Banned Hits (With Timer)" value={stats.banned.toLocaleString()} color="warning" />
                <StatRow label="Blocked (Security Alert)" value={stats.blocked.toLocaleString()} color="danger" />
              </>
            )}
            {banTab === 'donut' && (
              <>
                <div className="text-[9px] font-bold text-slate-500 uppercase tracking-widest mb-2">
                  donutsmp.net — via MC Protocol
                </div>
                <StatRow label="Unbanned on DonutSMP" value={(stats.donutUnbanned ?? 0).toLocaleString()} color="success" />
                <StatRow label="Banned on DonutSMP" value={(stats.donutBanned ?? 0).toLocaleString()} color="warning" />
                <StatRow label="Never Joined DonutSMP" value={(stats.donutNever ?? 0).toLocaleString()} color="default" />
                <StatRow label="Check Errors" value={(stats.donutError ?? 0).toLocaleString()} color="danger" />
              </>
            )}
            {banTab === 'minemen' && (
              <>
                <div className="text-[9px] font-bold text-slate-500 uppercase tracking-widest mb-2">
                  eu.minemen.club — via MC Protocol
                </div>
                <StatRow label="Unbanned on Minemen" value={(stats.minemenUnbanned ?? 0).toLocaleString()} color="success" />
                <StatRow label="Banned on Minemen" value={(stats.minemenBanned ?? 0).toLocaleString()} color="warning" />
                <StatRow label="Never Joined Minemen" value={(stats.minemenNever ?? 0).toLocaleString()} color="default" />
                <StatRow label="Check Errors" value={(stats.minemenError ?? 0).toLocaleString()} color="danger" />
              </>
            )}
          </div>
        </div>

        <StatGroup title="AUTH RESULTS">
          <StatRow label="Valid MS Hits" value={stats.hits.toLocaleString()} color="success" />
          <StatRow label="Discord Nitro Promos" value={stats.nitros.toLocaleString()} color="accent" />
          <StatRow label="No Minecraft" value={stats.noMC.toLocaleString()} color="default" />
          <StatRow label="Invalid Credentials" value={stats.invalid.toLocaleString()} color="danger" />
          <StatRow label="Rate Limited" value={(stats.rateLimited || 0).toLocaleString()} color="warning" />
          <StatRow label="Errors (network/other)" value={(stats.errors || 0).toLocaleString()} color="danger" />
        </StatGroup>

        <StatGroup title="NAME & SKIN REPLACEMENT">
          <StatRow label="Renamed (g00n_xxxx)" value={stats.renamed.toLocaleString()} color="success" />
          <StatRow label="Skin Applied (PNG)" value={stats.skinApplied.toLocaleString()} color="accent" />
          <StatRow label="Not Changeable (Cooldown)" value={stats.cooldown.toLocaleString()} color="warning" />
        </StatGroup>
      </div>

      <div className="bg-[#0e0e12]/85 border border-[#1f1f26] rounded-xl p-4 space-y-3 shadow-md backdrop-blur-md">
        <div className="flex items-center justify-between border-b border-[#16161c] pb-2">
          <div className="text-xs font-bold text-slate-200 uppercase tracking-wider">
            Detailed Check Log
          </div>
          <span className="text-[11px] text-slate-400 font-mono">
            last {logs.length} lines · pinned to bottom · full dump in results/.../errors.log
          </span>
        </div>
        <div
          ref={logBoxRef}
          className="h-72 overflow-y-auto font-mono text-[11px] space-y-0.5 bg-[#040406] border border-[#16161c] rounded-lg p-3"
        >
          {logs.length === 0 ? (
            <div className="text-slate-500 italic py-6 text-center">
              Per-account results will stream here when a run starts.
            </div>
          ) : (
            logs.map((log) => (
              <div key={log.id} className={`${logColor(log)} break-all leading-relaxed`}>
                <span className="text-slate-500 mr-2">[{log.timestamp}]</span>
                {log.message}
              </div>
            ))
          )}
          <div ref={logEndRef} />
        </div>
      </div>

      <div className="bg-[#0e0e12]/85 border border-[#1f1f26] rounded-xl p-4 space-y-3 shadow-md backdrop-blur-md">
        <div className="flex items-center justify-between border-b border-[#16161c] pb-2">
          <div className="text-xs font-bold text-slate-200 uppercase tracking-wider flex items-center gap-2">
            <span className="w-1.5 h-1.5 rounded-full bg-cyan-500 animate-pulse" />
            Live Real-time Check Feed
          </div>
          <span className="text-[11px] text-slate-400 font-mono">
            {liveHits.length} hits recorded in session
          </span>
        </div>

        <div className="overflow-x-auto max-h-80 overflow-y-auto">
          <table className="w-full text-left text-xs text-slate-200">
            <thead className="bg-[#14141a] text-[10px] text-slate-400 uppercase tracking-widest sticky top-0 z-10 border-b border-[#1f1f26]">
              <tr>
                <th className="px-4 py-2.5 font-bold">Time</th>
                <th className="px-4 py-2.5 font-bold">Hit ID</th>
                <th className="px-4 py-2.5 font-bold">Account / Email</th>
                <th className="px-4 py-2.5 font-bold">Hypixel Status</th>
                <th className="px-4 py-2.5 font-bold">Rename & Skin Details</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#16161c] font-mono text-[11px]">
              {liveHits.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-slate-500 italic">
                    {isChecking
                      ? 'Listening for hits... checks processing in background.'
                      : 'Click "Start Checking" to initiate live diagnostic check pipeline.'}
                  </td>
                </tr>
              ) : (
                liveHits.map((hit) => {
                  const badge = getStatusBadgeProps(hit.status, hit.banDuration);
                  return (
                    <tr key={hit.id} className="hover:bg-[#14141a]/60 transition-colors">
                      <td className="px-4 py-2 text-slate-500">{hit.timestamp}</td>
                      <td className="px-4 py-2 text-cyan-400 font-semibold">{hit.id}</td>
                      <td className="px-4 py-2 text-slate-200">
                        <span className="font-semibold">{hit.username}</span>
                        <span className="text-slate-500 ml-2">({hit.email})</span>
                      </td>
                      <td className="px-4 py-2">
                        <StatusBadge status={badge.type} label={badge.label} />
                      </td>
                      <td className="px-4 py-2 text-slate-400">{hit.renameStatus}</td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};
