import React from 'react';
import { Minus, Square, X, ShieldCheck } from 'lucide-react';
import { MinimiseWindow, ToggleMaximiseWindow, CloseWindow } from '../../../wailsjs/go/main/App';

export const WindowHeader: React.FC = () => {
  const handleMinimize = async () => {
    try {
      await MinimiseWindow();
    } catch {
      // fallback
    }
  };

  const handleMaximize = async () => {
    try {
      await ToggleMaximiseWindow();
    } catch {
      // fallback
    }
  };

  const handleClose = async () => {
    try {
      await CloseWindow();
    } catch {
      // fallback
    }
  };

  return (
    <header className="h-12 bg-[#09090b] border-b border-[#1f1f26] px-4 flex items-center justify-between select-none z-30 relative wails-drag">
      {/* Left: Branding */}
      <div className="flex items-center gap-2.5 wails-no-drag">
        <div className="p-1 bg-cyan-500/10 border border-cyan-500/20 rounded-md text-cyan-400">
          <ShieldCheck className="w-4 h-4" />
        </div>
        <span className="text-xs font-semibold text-slate-100 tracking-wider uppercase">
          Generic Checker
        </span>
      </div>

      {/* Center: Clean Space */}
      <div className="flex-1" />

      {/* Right: Window Controls */}
      <div className="flex items-center gap-1 wails-no-drag">
        <button
          onClick={handleMinimize}
          className="p-1.5 text-slate-400 hover:text-slate-100 hover:bg-neutral-800/50 rounded-md transition-all duration-200"
          title="Minimize"
        >
          <Minus className="w-3.5 h-3.5" />
        </button>
        <button
          onClick={handleMaximize}
          className="p-1.5 text-slate-400 hover:text-slate-100 hover:bg-neutral-800/50 rounded-md transition-all duration-200"
          title="Maximize"
        >
          <Square className="w-3 h-3" />
        </button>
        <button
          onClick={handleClose}
          className="p-1.5 text-slate-400 hover:text-white hover:bg-rose-500/90 rounded-md transition-all duration-200"
          title="Close"
        >
          <X className="w-3.5 h-3.5" />
        </button>
      </div>
    </header>
  );
};
