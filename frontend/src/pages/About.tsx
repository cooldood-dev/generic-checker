import React from 'react';
import { ShieldCheck, Cpu, Terminal, Layers } from 'lucide-react';

export const AboutPage: React.FC = () => {
  return (
    <div className="space-y-5 max-w-4xl mx-auto">
      <div>
        <h2 className="text-slate-100 text-lg font-bold uppercase tracking-wider">
          About Generic Checker
        </h2>
        <p className="text-xs text-slate-400 mt-0.5">
          High-performance diagnostic utility and session checker.
        </p>
      </div>

      <div className="bg-[#0e0e12] border border-[#1f1f26] rounded-xl p-6 space-y-5">
        <div className="flex items-center gap-3 border-b border-[#16161c] pb-4">
          <div className="p-2.5 bg-cyan-500/10 border border-cyan-500/20 rounded-xl text-cyan-400">
            <ShieldCheck className="w-6 h-6" />
          </div>
          <div>
            <h3 className="text-base font-bold text-slate-100">Generic Checker</h3>
            <span className="text-xs font-mono text-cyan-400">Version 1.0.0 (Wails v2 Desktop Build)</span>
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
          <div className="flex items-start gap-3 p-4 bg-[#14141a] rounded-lg border border-[#1f1f26] transition-all hover:border-cyan-500/30">
            <Cpu className="w-5 h-5 text-cyan-400 shrink-0 mt-0.5" />
            <div>
              <span className="font-bold text-slate-100 block">Go Backend Pipeline</span>
              <span className="text-slate-400">High-speed concurrent worker threads with native Microsoft OAuth & Mojang API handlers.</span>
            </div>
          </div>

          <div className="flex items-start gap-3 p-4 bg-[#14141a] rounded-lg border border-[#1f1f26] transition-all hover:border-cyan-500/30">
            <Layers className="w-5 h-5 text-cyan-400 shrink-0 mt-0.5" />
            <div>
              <span className="font-bold text-slate-100 block">React + Wails Bridge</span>
              <span className="text-slate-400">Componentry ASCII Canvas rendering & native IPC runtime bindings.</span>
            </div>
          </div>

          <div className="flex items-start gap-3 p-4 bg-[#14141a] rounded-lg border border-[#1f1f26] transition-all hover:border-cyan-500/30">
            <Terminal className="w-5 h-5 text-cyan-400 shrink-0 mt-0.5" />
            <div>
              <span className="font-bold text-slate-100 block">Auto Rename & Skin</span>
              <span className="text-slate-400">Automatically renames accounts to g00n_xxxx format & uploads local PNG skin before Hypixel check.</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
