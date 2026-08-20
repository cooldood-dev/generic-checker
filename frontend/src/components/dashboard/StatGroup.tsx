import React from 'react';

interface StatRowProps {
  label: string;
  value: string | number;
  highlight?: boolean;
  color?: 'default' | 'success' | 'warning' | 'danger' | 'accent';
}

export const StatRow: React.FC<StatRowProps> = ({
  label,
  value,
  highlight = false,
  color = 'default',
}) => {
  const getColorClass = () => {
    switch (color) {
      case 'success':
        return 'text-[#65D6A3]';
      case 'danger':
        return 'text-[#E76B7B]';
      case 'warning':
        return 'text-[#E8C86A]';
      case 'accent':
        return 'text-cyan-400';
      default:
        return highlight ? 'text-slate-100' : 'text-slate-400';
    }
  };

  return (
    <div className="flex items-center justify-between py-1.5 px-2 rounded-md hover:bg-[#14141a] transition-colors">
      <span className="text-xs font-medium text-slate-400">{label}</span>
      <span className={`text-xs font-semibold font-mono ${getColorClass()}`}>
        {value}
      </span>
    </div>
  );
};

interface StatGroupProps {
  title: string;
  children: React.ReactNode;
}

export const StatGroup: React.FC<StatGroupProps> = ({ title, children }) => {
  return (
    <div className="bg-[#0e0e12] border border-[#1f1f26] rounded-xl p-4 space-y-1.5 shadow-sm">
      <div className="text-[10px] font-bold text-slate-500 tracking-wider uppercase border-b border-[#16161c] pb-2 mb-2">
        {title}
      </div>
      <div className="space-y-0.5">{children}</div>
    </div>
  );
};
