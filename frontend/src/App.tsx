import React, { useState, useEffect } from 'react';
import { HashRouter, Routes, Route } from 'react-router-dom';
import { WindowHeader } from './components/layout/WindowHeader';
import { Sidebar } from './components/layout/Sidebar';
import { AsciiBackground } from './components/ascii/AsciiBackground';
import { LicenseScreen } from './components/loading/LicenseScreen';

import { CheckPage } from './pages/Check';
import { ConfigPage } from './pages/Config';
import { HitsPage } from './pages/Hits';
import { LiveLogsPage } from './pages/LiveLogs';
import { AboutPage } from './pages/About';
import { TelegramPage } from './pages/Telegram';
import { useAppStore } from './store/useAppStore';

export const App: React.FC = () => {
  const { initEvents, loadConfig } = useAppStore();
  const [isAuthenticated, setIsAuthenticated] = useState(false);

  useEffect(() => {
    initEvents();
    loadConfig();
  }, [initEvents, loadConfig]);

  return (
    <>
      {/* Launch / Authentication Screen */}
      {!isAuthenticated && (
        <LicenseScreen onComplete={() => setIsAuthenticated(true)} />
      )}

      {/* Main Wails Application Window */}
      <HashRouter>
        <div className="flex flex-col h-screen w-screen overflow-hidden bg-[#040406] text-slate-100 relative select-none rounded-lg border border-[#1f1f26]">
          {/* Componentry ASCII Cat Background Layer */}
          <AsciiBackground />

          {/* Custom Integrated Window Header (Frameless Title Bar) */}
          <WindowHeader />

          {/* Two-Column App Body */}
          <div className="flex flex-1 min-h-0 overflow-hidden relative z-10">
            {/* Dark Compact Sidebar */}
            <Sidebar />

            {/* Main Content Viewport */}
            <main className="flex-1 overflow-y-auto p-5 bg-[#040406]/75 backdrop-blur-[2px]">
              <Routes>
                <Route path="/" element={<CheckPage />} />
                <Route path="/config" element={<ConfigPage />} />
                <Route path="/telegram" element={<TelegramPage />} />
                <Route path="/hits" element={<HitsPage />} />
                <Route path="/logs" element={<LiveLogsPage />} />
                <Route path="/about" element={<AboutPage />} />
              </Routes>
            </main>
          </div>
        </div>
      </HashRouter>
    </>
  );
};

export default App;
