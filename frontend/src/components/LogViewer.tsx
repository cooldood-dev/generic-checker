import React, { useState, useEffect, useRef } from 'react';
import { Search, ArrowDown } from 'lucide-react';
import { useAppStore, LogEntry } from '../store/useAppStore';

export const LogViewer: React.FC = () => {
  const { logs } = useAppStore();
  const [filter, setFilter] = useState('');
  const [autoScroll, setAutoScroll] = useState(true);
  const boxRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!autoScroll) return;
    const box = boxRef.current;
    if (box) box.scrollTop = box.scrollHeight;
  }, [logs, autoScroll]);

  const filteredLogs = logs.filter(
    (log) =>
      log.message.toLowerCase().includes(filter.toLowerCase()) ||
      log.level.toLowerCase().includes(filter.toLowerCase())
  );

  const getLevelColor = (level: LogEntry['level']) => {
    switch (level) {
      case 'info':
        return 'text-cyan-400';
      case 'warn':
        return 'text-amber-400';
      case 'error':
        return 'text-rose-400';
      case 'success':
        return 'text-emerald-400';
      default:
        return 'text-slate-400';
    }
  };

  return (
    <div className="bg-[#0e0e12] border border-[#1f1f26] rounded-xl flex flex-col h-[calc(100vh-140px)] shadow-md">
      <div className="p-4 border-b border-[#16161c] flex items-center justify-between gap-4">
        <div className="relative flex-1 max-w-md">
          <Search className="w-4 h-4 absolute left-3 top-2.5 text-slate-500" />
          <input
            type="text"
            placeholder="Filter: INVALID, RATE LIMITED, HIT…"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="w-full bg-[#040406] border border-[#1f1f26] rounded-md pl-9 pr-4 py-1.5 text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-cyan-500 transition-all duration-200"
          />
        </div>
        <button
          onClick={() => setAutoScroll(!autoScroll)}
          className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium border transition-all duration-200 cursor-pointer ${
            autoScroll
              ? 'bg-cyan-500/10 text-cyan-400 border-cyan-500/25'
              : 'bg-[#14141a] text-slate-400 border-[#1f1f26]'
          }`}
        >
          <ArrowDown className={`w-3.5 h-3.5 ${autoScroll ? 'animate-pulse' : ''}`} />
          Stick to bottom
        </button>
      </div>

      <div ref={boxRef} className="flex-1 p-4 overflow-y-auto font-mono text-xs space-y-1 bg-[#040406]/80">
        {filteredLogs.length === 0 ? (
          <div className="text-slate-500 py-8 text-center italic">No log entries match your filter.</div>
        ) : (
          filteredLogs.map((log) => (
            <div
              key={log.id}
              className="flex items-start gap-3 py-0.5 hover:bg-[#14141a]/60 px-2 rounded-md transition-colors"
            >
              <span className="text-slate-500 shrink-0 select-none">[{log.timestamp}]</span>
              <span
                className={`font-semibold uppercase tracking-wider text-[10px] shrink-0 px-1.5 py-0.5 rounded bg-[#0e0e12] border border-[#1f1f26] ${getLevelColor(
                  log.level
                )}`}
              >
                {log.level}
              </span>
              <span className={`break-all leading-relaxed ${getLevelColor(log.level)}`}>
                {log.message}
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  );
};
