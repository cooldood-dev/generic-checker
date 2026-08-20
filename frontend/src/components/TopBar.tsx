import React, { useEffect } from 'react';
import { Settings, RefreshCw, Sun, Moon } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { useAppStore } from '../store/useAppStore';

export const TopBar: React.FC = () => {
  const navigate = useNavigate();
  const { systemStatus, fetchSystemStatus, theme, toggleTheme } = useAppStore();

  useEffect(() => {
    fetchSystemStatus();
    const interval = setInterval(fetchSystemStatus, 10000);
    return () => clearInterval(interval);
  }, [fetchSystemStatus]);

  const getStatusColor = () => {
    switch (systemStatus.status) {
      case 'healthy':
        return 'bg-emerald-500 shadow-emerald-500/50';
      case 'warning':
        return 'bg-amber-500 shadow-amber-500/50';
      case 'error':
        return 'bg-rose-500 shadow-rose-500/50';
      default:
        return 'bg-slate-500';
    }
  };

  return (
    <header className="h-16 bg-slate-900/90 backdrop-blur-md border-b border-slate-800 px-6 flex items-center justify-between sticky top-0 z-10">
      {/* Left: App Title */}
      <div className="flex items-center gap-3">
        <h1 className="text-base font-semibold text-slate-100 tracking-tight">
          Generic Checker
        </h1>
        <span className="px-2 py-0.5 rounded text-[10px] font-mono font-medium bg-slate-800 text-slate-400 border border-slate-700">
          v1.0.0
        </span>
      </div>

      {/* Right: Status Indicator & Controls */}
      <div className="flex items-center gap-4">
        {/* System Health Dot */}
        <div
          className="flex items-center gap-2 px-3 py-1 bg-slate-800/80 rounded-full border border-slate-700/60 text-xs text-slate-300"
          title={`Uptime: ${systemStatus.uptime}`}
        >
          <span
            className={`w-2.5 h-2.5 rounded-full ${getStatusColor()} shadow-sm animate-pulse`}
          />
          <span className="font-medium capitalize">{systemStatus.status}</span>
        </div>

        {/* Manual Refresh */}
        <button
          onClick={() => fetchSystemStatus()}
          className="p-2 text-slate-400 hover:text-slate-200 hover:bg-slate-800 rounded-lg transition-colors"
          title="Refresh Status"
        >
          <RefreshCw className="w-4 h-4" />
        </button>

        {/* Dark/Light Toggle */}
        <button
          onClick={toggleTheme}
          className="p-2 text-slate-400 hover:text-slate-200 hover:bg-slate-800 rounded-lg transition-colors"
          title="Toggle Theme"
        >
          {theme === 'dark' ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
        </button>

        {/* Settings Icon */}
        <button
          onClick={() => navigate('/settings')}
          className="p-2 text-slate-400 hover:text-slate-200 hover:bg-slate-800 rounded-lg transition-colors"
          title="Open Settings"
        >
          <Settings className="w-4 h-4" />
        </button>
      </div>
    </header>
  );
};
