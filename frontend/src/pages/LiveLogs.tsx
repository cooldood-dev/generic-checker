import React from 'react';
import { LogViewer } from '../components/LogViewer';

export const LiveLogsPage: React.FC = () => {
  return (
    <div className="space-y-4 max-w-7xl mx-auto">
      <div>
        <h2 className="text-[#F3F3F5] text-lg font-bold uppercase tracking-wider">
          Live System Terminal Logs
        </h2>
        <p className="text-xs text-[#85858D] mt-0.5">
          Realtime output stream from the Go backend, Wails events, and checker diagnostics.
        </p>
      </div>

      <LogViewer />
    </div>
  );
};
