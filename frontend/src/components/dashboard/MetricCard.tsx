import React from 'react';

interface MetricCardProps {
  label: string;
  value: string;
  subtext?: string;
  highlight?: boolean;
}

export const MetricCard: React.FC<MetricCardProps> = ({
  label,
  value,
  subtext,
  highlight = false,
}) => {
  return (
    <div
      className={`bg-[#0e0e12] border ${
        highlight ? 'border-cyan-500/40 bg-cyan-500/5 shadow-sm shadow-cyan-500/5' : 'border-[#1f1f26] hover:border-[#2d2d38]'
      } rounded-xl p-5 transition-all duration-200`}
    >
      <div className="text-[10px] font-semibold text-slate-400 tracking-wider uppercase mb-1">
        {label}
      </div>
      <div className="text-3xl font-bold text-slate-100 tracking-tight">
        {value}
      </div>
      {subtext && (
        <div className="text-[11px] text-slate-500 mt-1">
          {subtext}
        </div>
      )}
    </div>
  );
};
