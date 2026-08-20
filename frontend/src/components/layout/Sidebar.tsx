import React from 'react';
import { NavLink } from 'react-router-dom';
import {
  CheckSquare,
  Sliders,
  Award,
  Terminal,
  Info,
  Send,
} from 'lucide-react';

interface NavSection {
  title: string;
  items: {
    to: string;
    label: string;
    icon: React.ComponentType<{ className?: string }>;
  }[];
}

export const Sidebar: React.FC = () => {
  const sections: NavSection[] = [
    {
      title: 'CHECKER',
      items: [
        { to: '/', label: 'Check', icon: CheckSquare },
        { to: '/config', label: 'Config', icon: Sliders },
        { to: '/telegram', label: 'Telegram Auto', icon: Send },
      ],
    },
    {
      title: 'RESULTS',
      items: [
        { to: '/hits', label: 'Hits', icon: Award },
        { to: '/logs', label: 'Live Logs', icon: Terminal },
      ],
    },
    {
      title: 'SYSTEM',
      items: [{ to: '/about', label: 'About', icon: Info }],
    },
  ];

  return (
    <aside className="w-44 bg-[#09090b] border-r border-[#1f1f26] flex flex-col z-20 shrink-0 select-none">
      <nav className="flex-1 py-4 px-2 space-y-6 overflow-y-auto">
        {sections.map((section) => (
          <div key={section.title} className="space-y-1">
            {/* Section Header */}
            <div className="px-2 pb-1 text-[10px] font-bold text-slate-500 tracking-wider uppercase">
              {section.title}
            </div>

            {/* Nav Items */}
            {section.items.map((item) => {
              const Icon = item.icon;
              return (
                <NavLink
                  key={item.to}
                  to={item.to}
                  className={({ isActive }) =>
                    `flex items-center gap-2.5 px-2.5 py-1.5 rounded-md text-xs font-medium transition-all duration-200 ${
                      isActive
                        ? 'bg-cyan-500/10 text-cyan-400 border-l-2 border-cyan-500 font-semibold'
                        : 'text-slate-400 hover:text-slate-100 hover:bg-[#14141a]'
                    }`
                  }
                >
                  <Icon className="w-4 h-4 shrink-0" />
                  <span>{item.label}</span>
                </NavLink>
              );
            })}
          </div>
        ))}
      </nav>
    </aside>
  );
};
