import React from 'react';

export type StatusType = 'pass' | 'fail' | 'pending' | 'warning' | 'idle' | 'healthy' | 'error';

interface StatusBadgeProps {
  status: StatusType;
  label?: string;
}

export const StatusBadge: React.FC<StatusBadgeProps> = ({ status, label }) => {
  const displayLabel = label || status.toUpperCase();

  const getStyles = () => {
    switch (status) {
      case 'pass':
      case 'healthy':
        return 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20';
      case 'fail':
      case 'error':
        return 'bg-rose-500/10 text-rose-400 border-rose-500/20';
      case 'warning':
        return 'bg-amber-500/10 text-amber-400 border-amber-500/20';
      case 'pending':
        return 'bg-sky-500/10 text-sky-400 border-sky-500/20 animate-pulse';
      case 'idle':
      default:
        return 'bg-slate-500/10 text-slate-400 border-slate-500/20';
    }
  };

  return (
    <span
      className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold border ${getStyles()}`}
    >
      <span className="w-1.5 h-1.5 rounded-full bg-current" />
      {displayLabel}
    </span>
  );
};
