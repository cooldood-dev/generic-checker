import { create } from 'zustand';
import {
  GetSystemStatus,
  StartChecking,
  StopChecking,
  GetConfig,
  SaveConfig,
  OpenResults,
  TelegramSendCode,
  TelegramConfirmCode,
  TelegramLogout,
  TelegramFetchChannels,
  TelegramStartMonitor,
  TelegramStopMonitor,
  TelegramStatus,
  TelegramOpenTempDir,
  TelegramGetConfig,
  TelegramSaveCredentials,
} from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';

export interface TelegramChannel {
  id: number;
  accessHash: number;
  title: string;
  username: string;
  broadcast: boolean;
  megagroup: boolean;
}

export interface TelegramMonitorTarget {
  id: number;
  accessHash: number;
  title: string;
  broadcast: boolean;
}

export interface TelegramDownload {
  channel: string;
  fileName: string;
  filePath: string;
  extractedTo: string;
  password: string;
  extractOK: boolean;
  messageText: string;
}

export interface TelegramState {
  configured: boolean;
  loggedIn: boolean;
  monitoring: boolean;
  user: string;
  phone: string;
}

export interface LiveHit {
  id: string;
  timestamp: string;
  email: string;
  username: string;
  newUsername?: string;
  status: 'unbanned' | 'banned' | 'blocked' | 'no_mc' | 'invalid';
  banDuration?: string;
  renameStatus: string;
}

export interface CheckStats {
  total: number;
  processed: number;
  hits: number;
  unbanned: number;
  banned: number;
  blocked: number;
  noMC: number;
  invalid: number;
  rateLimited: number;
  errors: number;
  renamed: number;
  skinApplied: number;
  cooldown: number;
  nitros: number;
  cpm: number;
  elapsedSeconds: number;
  donutUnbanned: number;
  donutBanned: number;
  donutNever: number;
  donutError: number;
  minemenUnbanned: number;
  minemenBanned: number;
  minemenNever: number;
  minemenError: number;
}

export interface SystemStatus {
  status: 'healthy' | 'warning' | 'error';
  uptime: string;
  lastChecked: string;
}

export interface LogEntry {
  id: string;
  timestamp: string;
  level: 'info' | 'warn' | 'error' | 'success';
  message: string;
}

export interface CheckConfig {
  cookiesDir: string;
  comboPath: string;
  ulpPath: string;
  proxyFile: string;
  workers: number;
  timeoutSec: number;
  doHypixel: boolean;
  doDonut: boolean;
  doRename: boolean;
  skinFile: string;
  skinVariant: string;
  allFiles: boolean;
  noCookies: boolean;
  noCreds: boolean;
  ulpAll: boolean;
  relocate: string;
  runName: string;
  discordWebhook: string;
  doUpload: boolean;
}

interface AppState {
  sidebarCollapsed: boolean;
  theme: 'dark' | 'light';
  isChecking: boolean;
  systemStatus: SystemStatus;
  stats: CheckStats;
  liveHits: LiveHit[];
  logs: LogEntry[];
  config: CheckConfig;

  tgState: TelegramState;
  tgChannels: TelegramChannel[];
  tgSelected: number[];
  tgKeywords: string;
  tgLogs: LogEntry[];
  tgDownloads: TelegramDownload[];
  tgCodeSent: boolean;
  tgBusy: boolean;
  tgError: string;

  toggleSidebar: () => void;
  toggleTheme: () => void;
  fetchSystemStatus: () => Promise<void>;
  loadConfig: () => Promise<void>;
  saveConfig: (cfg: Partial<CheckConfig>) => Promise<void>;
  startChecking: () => Promise<void>;
  stopChecking: () => Promise<void>;
  openResults: () => Promise<void>;
  addLog: (level: LogEntry['level'], message: string) => void;
  initEvents: () => void;

  tgInit: () => Promise<void>;
  tgSendCode: (apiId: number, apiHash: string, phone: string) => Promise<boolean>;
  tgConfirmCode: (code: string, password: string) => Promise<boolean>;
  tgLogout: () => Promise<boolean>;
  tgFetchChannels: () => Promise<void>;
  tgToggleChannel: (id: number) => void;
  tgSetKeywords: (kw: string) => void;
  tgStartMonitor: () => Promise<void>;
  tgStopMonitor: () => Promise<void>;
  tgOpenTempDir: () => Promise<void>;
  tgAddLog: (level: LogEntry['level'], message: string) => void;
  tgClearLogs: () => void;
  tgClearError: () => void;
}

const MAX_LOGS = 120;
const MAX_HITS = 40;

const initialStats: CheckStats = {
  total: 0,
  processed: 0,
  hits: 0,
  unbanned: 0,
  banned: 0,
  blocked: 0,
  noMC: 0,
  invalid: 0,
  rateLimited: 0,
  errors: 0,
  renamed: 0,
  skinApplied: 0,
  cooldown: 0,
  nitros: 0,
  cpm: 0,
  elapsedSeconds: 0,
  donutUnbanned: 0,
  donutBanned: 0,
  donutNever: 0,
  donutError: 0,
  minemenUnbanned: 0,
  minemenBanned: 0,
  minemenNever: 0,
  minemenError: 0,
};

const defaultConfig: CheckConfig = {
  cookiesDir: 'cookies',
  comboPath: '',
  ulpPath: '',
  proxyFile: 'proxies.txt',
  workers: 250,
  timeoutSec: 25,
  doHypixel: true,
  doDonut: false,
  doRename: true,
  skinFile: '',
  skinVariant: 'classic',
  allFiles: false,
  noCookies: false,
  noCreds: false,
  ulpAll: false,
  relocate: 'copy',
  runName: '',
  discordWebhook: '',
  doUpload: false,
};

let eventsReady = false;
let logSeq = 0;

function trimLogs(logs: LogEntry[]): LogEntry[] {
  if (logs.length <= MAX_LOGS) return logs;
  return logs.slice(logs.length - MAX_LOGS);
}

export const useAppStore = create<AppState>((set, get) => ({
  sidebarCollapsed: false,
  theme: 'dark',
  isChecking: false,
  systemStatus: {
    status: 'healthy',
    uptime: '00:00:00',
    lastChecked: 'Just now',
  },
  stats: initialStats,
  liveHits: [],
  logs: [
    { id: '1', timestamp: new Date().toLocaleTimeString(), level: 'info', message: 'Generic Checker GUI ready.' },
  ],
  config: defaultConfig,

  tgState: { configured: false, loggedIn: false, monitoring: false, user: '', phone: '' },
  tgChannels: [],
  tgSelected: [],
  tgKeywords: '',
  tgLogs: [],
  tgDownloads: [],
  tgCodeSent: false,
  tgBusy: false,
  tgError: '',

  toggleSidebar: () => set((state) => ({ sidebarCollapsed: !state.sidebarCollapsed })),
  toggleTheme: () => set((state) => ({ theme: state.theme === 'dark' ? 'light' : 'dark' })),

  fetchSystemStatus: async () => {
    try {
      const res = await GetSystemStatus();
      set({
        systemStatus: {
          status: (res.status as 'healthy' | 'warning' | 'error') || 'healthy',
          uptime: res.uptime || '0s',
          lastChecked: res.lastChecked || new Date().toLocaleTimeString(),
        },
      });
    } catch (err) {
      console.error('Failed to fetch status:', err);
    }
  },

  loadConfig: async () => {
    try {
      const cfg = await GetConfig();
      set({
        config: {
          ...defaultConfig,
          ...cfg,
          cookiesDir: cfg.cookiesDir || 'cookies',
          proxyFile: cfg.proxyFile || 'proxies.txt',
          workers: cfg.workers || 250,
          timeoutSec: cfg.timeoutSec || 25,
          skinVariant: cfg.skinVariant || 'classic',
          relocate: cfg.relocate || 'copy',
        },
      });
    } catch (err) {
      console.error('Failed to load config:', err);
    }
  },

  saveConfig: async (partial) => {
    const next = { ...get().config, ...partial };
    try {
      const saved = await SaveConfig(next);
      set({ config: { ...defaultConfig, ...saved } });
      get().addLog('info', 'Updated configuration flags & skin parameters.');
    } catch (err) {
      console.error('Failed to save config:', err);
      get().addLog('error', 'Failed to save configuration.');
    }
  },

  startChecking: async () => {
    if (get().isChecking) return;
    set({
      isChecking: true,
      stats: { ...initialStats },
      liveHits: [],
      logs: [
        {
          id: 'run-start',
          timestamp: new Date().toLocaleTimeString(),
          level: 'info',
          message: 'Started checking — detailed lines stream below (invalids sampled in UI; full errors.log on disk).',
        },
      ],
    });
    try {
      await StartChecking(get().config);
    } catch (err: any) {
      set({ isChecking: false });
      get().addLog('error', err?.message || String(err) || 'Failed to start checking');
    }
  },

  stopChecking: async () => {
    try {
      await StopChecking();
    } catch (err) {
      console.error(err);
    }
  },

  openResults: async () => {
    try {
      await OpenResults();
    } catch (err: any) {
      get().addLog('warn', err?.message || 'No results directory yet.');
    }
  },

  addLog: (level, message) => {
    const newLog: LogEntry = {
      id: `local-${++logSeq}`,
      timestamp: new Date().toLocaleTimeString(),
      level,
      message,
    };
    // Newest at bottom
    set((state) => ({ logs: trimLogs([...state.logs, newLog]) }));
  },

  tgAddLog: (level, message) => {
    const entry: LogEntry = {
      id: `tg-${++logSeq}`,
      timestamp: new Date().toLocaleTimeString(),
      level,
      message,
    };
    set((state) => ({ tgLogs: [...state.tgLogs, entry].slice(-300) }));
  },

  tgClearLogs: () => set({ tgLogs: [] }),

  tgClearError: () => set({ tgError: '' }),

  tgInit: async () => {
    try {
      const st = await TelegramStatus();
      const cfg = await TelegramGetConfig();
      set({
        tgState: {
          configured: st.configured,
          loggedIn: st.loggedIn,
          monitoring: st.monitoring,
          user: st.user,
          phone: st.phone,
        },
        tgChannels: get().tgChannels.length
          ? get().tgChannels
          : cfg.channels.length
            ? cfg.channels.map((c) => ({
                id: c.id,
                accessHash: c.accessHash,
                title: c.title,
                username: '',
                broadcast: c.broadcast,
                megagroup: false,
              }))
            : [],
        tgSelected: cfg.channels.length ? cfg.channels.map((c) => c.id) : get().tgSelected,
        tgKeywords: cfg.keywords.join(', '),
      });
    } catch (err) {
      console.error('tgInit failed:', err);
    }
  },

  tgSendCode: async (apiId, apiHash, phone) => {
    set({ tgBusy: true, tgError: '' });
    try {
      await TelegramSaveCredentials(apiId, apiHash, phone);
      await TelegramSendCode();
      set({ tgCodeSent: true, tgState: { ...get().tgState, configured: true, phone } });
      get().tgAddLog('info', 'Verification code sent to Telegram.');
      return true;
    } catch (err: any) {
      const msg = err?.message || String(err) || 'Failed to send code';
      if (msg.toLowerCase().includes('already logged in')) {
        get().tgAddLog('info', 'Session is already authorized.');
        set({ tgCodeSent: false });
        await get().tgInit();
        return true;
      }
      get().tgAddLog('error', msg);
      set({ tgError: msg });
      return false;
    } finally {
      set({ tgBusy: false });
    }
  },

  tgConfirmCode: async (code, password) => {
    set({ tgBusy: true, tgError: '' });
    try {
      await TelegramConfirmCode(code, password);
      set({ tgCodeSent: false });
      get().tgAddLog('success', 'Logged in to Telegram.');
      await get().tgInit();
      return true;
    } catch (err: any) {
      const msg = err?.message || String(err) || 'Login failed';
      get().tgAddLog('error', msg);
      if (msg.toLowerCase().includes('expired') || msg.toLowerCase().includes('auth_restart')) {
        // Telegram invalidated the code hash — ask the user to send a new code.
        set({ tgCodeSent: false });
      }
      set({ tgError: msg });
      return false;
    } finally {
      set({ tgBusy: false });
    }
  },

  tgLogout: async () => {
    set({ tgBusy: true });
    try {
      await TelegramLogout();
      set({ tgCodeSent: false });
      get().tgAddLog('info', 'Logged out.');
      await get().tgInit();
      return true;
    } catch (err: any) {
      const msg = err?.message || String(err) || 'Logout failed';
      get().tgAddLog('error', msg);
      set({ tgError: msg });
      return false;
    } finally {
      set({ tgBusy: false });
    }
  },

  tgFetchChannels: async () => {
    set({ tgBusy: true });
    try {
      const channels = await TelegramFetchChannels();
      set({ tgChannels: channels });
      get().tgAddLog('info', `Loaded ${channels.length} channel(s).`);
    } catch (err: any) {
      get().tgAddLog('error', err?.message || String(err) || 'Failed to load channels');
    } finally {
      set({ tgBusy: false });
    }
  },

  tgToggleChannel: (id) => {
    set((state) => ({
      tgSelected: state.tgSelected.includes(id)
        ? state.tgSelected.filter((x) => x !== id)
        : [...state.tgSelected, id],
    }));
  },

  tgSetKeywords: (kw) => set({ tgKeywords: kw }),

  tgStartMonitor: async () => {
    const channels = get().tgChannels
      .filter((c) => get().tgSelected.includes(c.id))
      .map((c): TelegramMonitorTarget => ({
        id: c.id,
        accessHash: c.accessHash,
        title: c.title,
        broadcast: c.broadcast,
      }));
    const keywords = get().tgKeywords
      .split(',')
      .map((k) => k.trim())
      .filter(Boolean);
    if (!channels.length) {
      get().tgAddLog('warn', 'Select at least one channel.');
      return;
    }
    if (!keywords.length) {
      get().tgAddLog('warn', 'Enter at least one keyword.');
      return;
    }
    set({ tgBusy: true });
    try {
      await TelegramStartMonitor(channels, keywords);
      get().tgAddLog('success', `Monitoring ${channels.length} channel(s) for: ${keywords.join(', ')}`);
      await get().tgInit();
    } catch (err: any) {
      get().tgAddLog('error', err?.message || String(err) || 'Failed to start monitor');
    } finally {
      set({ tgBusy: false });
    }
  },

  tgStopMonitor: async () => {
    set({ tgBusy: true });
    try {
      await TelegramStopMonitor();
      get().tgAddLog('info', 'Monitoring stopped.');
      await get().tgInit();
    } catch (err: any) {
      get().tgAddLog('error', err?.message || String(err) || 'Failed to stop monitor');
    } finally {
      set({ tgBusy: false });
    }
  },

  tgOpenTempDir: async () => {
    try {
      await TelegramOpenTempDir();
    } catch (err: any) {
      get().tgAddLog('warn', err?.message || 'Failed to open temp directory');
    }
  },

  initEvents: () => {
    if (eventsReady) return;
    eventsReady = true;

    EventsOn('stats:update', (data: CheckStats) => {
      if (!data) return;
      set((state) => ({
        stats: {
          ...state.stats,
          ...data,
          cpm: typeof data.cpm === 'number' ? Math.round(data.cpm) : state.stats.cpm,
        },
      }));
    });

    EventsOn('hit:new', (hit: LiveHit) => {
      if (!hit) return;
      set((state) => ({
        liveHits: [hit, ...state.liveHits].slice(0, MAX_HITS),
      }));
    });

    // Batched logs from Go (preferred)
    EventsOn('log:batch', (batch: LogEntry[] | any) => {
      const rows: LogEntry[] = Array.isArray(batch) ? batch : [];
      if (!rows.length) return;
      const mapped = rows.map((entry, i) => ({
        id: entry.id || `b-${++logSeq}-${i}`,
        timestamp: entry.timestamp || new Date().toLocaleTimeString(),
        level: (entry.level as LogEntry['level']) || 'info',
        message: entry.message || '',
      })).filter((e) => e.message);
      if (!mapped.length) return;
      set((state) => ({ logs: trimLogs([...state.logs, ...mapped]) }));
    });

    // Legacy single-line events (still supported)
    EventsOn('log:line', (entry: LogEntry) => {
      if (!entry?.message) return;
      set((state) => ({
        logs: trimLogs([
          ...state.logs,
          {
            id: entry.id || `l-${++logSeq}`,
            timestamp: entry.timestamp || new Date().toLocaleTimeString(),
            level: entry.level || 'info',
            message: entry.message,
          },
        ]),
      }));
    });

    EventsOn('run:done', () => {
      set({ isChecking: false });
    });

    EventsOn('tg:log', (entry: LogEntry | any) => {
      if (!entry?.message) return;
      const level = (['info', 'warn', 'error', 'success'] as const).includes(entry.level)
        ? entry.level
        : 'info';
      set((state) => ({
        tgLogs: [
          ...state.tgLogs,
          {
            id: entry.id || `tg-${++logSeq}`,
            timestamp: entry.timestamp || new Date().toLocaleTimeString(),
            level,
            message: entry.message,
          },
        ].slice(-300),
      }));
    });

    EventsOn('tg:status', (st: TelegramState | any) => {
      if (!st) return;
      set((state) => ({
        tgState: {
          configured: !!st.configured,
          loggedIn: !!st.loggedIn,
          monitoring: !!st.monitoring,
          user: st.user || '',
          phone: st.phone || '',
        },
      }));
    });

    EventsOn('tg:download', (info: TelegramDownload | any) => {
      if (!info) return;
      set((state) => ({
        tgDownloads: [info, ...state.tgDownloads].slice(0, 200),
      }));
    });
  },
}));
