import React from 'react';
import { StatusBadge } from '../components/StatusBadge';
import { useAppStore } from '../store/useAppStore';

export const HitsPage: React.FC = () => {
  const { liveHits } = useAppStore();

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
    <div className="space-y-4 max-w-7xl mx-auto">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-slate-100 text-lg font-bold uppercase tracking-wider">
            Diagnostic Hits Log
          </h2>
          <p className="text-xs text-slate-400 mt-0.5">
            Recorded hit accounts, Hypixel ban status, and username replacement outcomes.
          </p>
        </div>
        <div className="text-xs text-cyan-400 font-mono font-semibold">
          {liveHits.length} Hits Recorded
        </div>
      </div>

      <div className="bg-[#0e0e12]/85 border border-[#1f1f26] rounded-xl overflow-hidden shadow-md backdrop-blur-md">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs text-slate-200">
            <thead className="bg-[#14141a] text-[10px] text-slate-400 uppercase tracking-widest border-b border-[#1f1f26]">
              <tr>
                <th className="px-5 py-3 font-bold">Hit ID</th>
                <th className="px-5 py-3 font-bold">Account / Email</th>
                <th className="px-5 py-3 font-bold">Hypixel Status</th>
                <th className="px-5 py-3 font-bold">Rename & Skin Outcome</th>
                <th className="px-5 py-3 font-bold text-right">Timestamp</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#16161c] font-mono text-xs">
              {liveHits.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-5 py-10 text-center text-slate-500 italic">
                    No hits recorded yet. Start a check run from the Check main page to capture live hits.
                  </td>
                </tr>
              ) : (
                liveHits.map((hit) => {
                  const badge = getStatusBadgeProps(hit.status, hit.banDuration);
                  return (
                    <tr key={hit.id} className="hover:bg-[#14141a]/60 transition-all duration-150">
                      <td className="px-5 py-3.5 text-cyan-400 font-semibold">{hit.id}</td>
                      <td className="px-5 py-3.5">
                        <div className="font-bold text-slate-200">{hit.username}</div>
                        <div className="text-[11px] text-slate-400">{hit.email}</div>
                      </td>
                      <td className="px-5 py-3.5">
                        <StatusBadge status={badge.type} label={badge.label} />
                      </td>
                      <td className="px-5 py-3.5 text-slate-400">{hit.renameStatus}</td>
                      <td className="px-5 py-3.5 text-right text-slate-500">{hit.timestamp}</td>
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
