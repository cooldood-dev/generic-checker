import React from 'react';

interface ProgressBarProps {
  progress: number; // 0 to 100
  label?: string;
}

export const ProgressBar: React.FC<ProgressBarProps> = ({ progress, label = 'Progress' }) => {
  const percentage = Math.min(100, Math.max(0, progress));

  return (
    <div className="space-y-1.5 w-full">
      <div className="flex items-center justify-between text-xs text-slate-400 font-mono">
        <span>{label}</span>
        <span className="font-semibold text-slate-200">{percentage.toFixed(1)}%</span>
      </div>
      <div className="w-full h-2 bg-[#14141a] rounded-full overflow-hidden border border-[#1f1f26]">
        <div
          className="h-full bg-cyan-500 transition-all duration-300 rounded-full"
          style={{ width: `${percentage}%` }}
        />
      </div>
    </div>
  );
};
