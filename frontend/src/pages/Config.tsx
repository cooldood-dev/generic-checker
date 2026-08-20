import React, { useEffect, useState } from 'react';
import { Save, Folder, Shield, UserCheck, Image, Send } from 'lucide-react';
import { useAppStore } from '../store/useAppStore';

export const ConfigPage: React.FC = () => {
  const { config, loadConfig, saveConfig } = useAppStore();
  const [cookiesDir, setCookiesDir] = useState(config.cookiesDir);
  const [comboPath, setComboPath] = useState(config.comboPath);
  const [proxyFile, setProxyFile] = useState(config.proxyFile);
  const [workers, setWorkers] = useState(String(config.workers));
  const [timeout, setTimeoutVal] = useState(String(config.timeoutSec));
  const [doHypixel, setDoHypixel] = useState(config.doHypixel);
  const [doDonut, setDoDonut] = useState(config.doDonut ?? false);
  const [doRename, setDoRename] = useState(config.doRename);
  const [skinFile, setSkinFile] = useState(config.skinFile);
  const [skinVariant, setSkinVariant] = useState(config.skinVariant);
  const [discordWebhook, setDiscordWebhook] = useState(config.discordWebhook || '');
  const [doUpload, setDoUpload] = useState(config.doUpload || false);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    loadConfig();
  }, [loadConfig]);

  useEffect(() => {
    setCookiesDir(config.cookiesDir);
    setComboPath(config.comboPath);
    setProxyFile(config.proxyFile);
    setWorkers(String(config.workers));
    setTimeoutVal(String(config.timeoutSec));
    setDoHypixel(config.doHypixel);
    setDoDonut(config.doDonut ?? false);
    setDoRename(config.doRename);
    setSkinFile(config.skinFile || '');
    setSkinVariant(config.skinVariant || 'classic');
    setDiscordWebhook(config.discordWebhook || '');
    setDoUpload(config.doUpload || false);
  }, [config]);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    await saveConfig({
      cookiesDir,
      comboPath,
      proxyFile,
      workers: parseInt(workers, 10) || 250,
      timeoutSec: parseInt(timeout, 10) || 25,
      doHypixel,
      doDonut,
      doRename,
      skinFile,
      skinVariant,
      discordWebhook,
      doUpload,
    });
    setSaved(true);
    setTimeout(() => setSaved(false), 2500);
  };

  return (
    <div className="space-y-5 max-w-5xl mx-auto">
      <div>
        <h2 className="text-slate-100 text-lg font-bold uppercase tracking-wider">
          Configuration
        </h2>
        <p className="text-xs text-slate-400 mt-0.5">
          Tune diagnostic parameters, file paths, proxy settings, and skin auto-replacement.
        </p>
      </div>

      <form onSubmit={handleSave} className="space-y-4">
        <div className="bg-[#0e0e12] border border-[#1f1f26] rounded-xl p-5 space-y-4">
          <div className="flex items-center gap-2 text-xs font-bold text-slate-200 uppercase tracking-wider border-b border-[#16161c] pb-2">
            <Folder className="w-4 h-4 text-cyan-500" />
            Input & Directory Sources
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
            <div>
              <label className="block text-slate-400 font-medium mb-1">
                Cookies Directory (-dir)
              </label>
              <input
                type="text"
                value={cookiesDir}
                onChange={(e) => setCookiesDir(e.target.value)}
                className="w-full bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200"
              />
            </div>

            <div>
              <label className="block text-slate-400 font-medium mb-1">
                Extra Combo Path (-combo)
              </label>
              <input
                type="text"
                value={comboPath}
                placeholder="Optional path to combolist"
                onChange={(e) => setComboPath(e.target.value)}
                className="w-full bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200"
              />
            </div>
          </div>
        </div>

        <div className="bg-[#0e0e12] border border-[#1f1f26] rounded-xl p-5 space-y-4">
          <div className="flex items-center gap-2 text-xs font-bold text-slate-200 uppercase tracking-wider border-b border-[#16161c] pb-2">
            <Shield className="w-4 h-4 text-cyan-500" />
            Network & Performance
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 text-xs">
            <div>
              <label className="block text-slate-400 font-medium mb-1">
                Concurrent Workers (-w)
              </label>
              <input
                type="number"
                value={workers}
                onChange={(e) => setWorkers(e.target.value)}
                className="w-full bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200"
              />
            </div>

            <div>
              <label className="block text-slate-400 font-medium mb-1">
                HTTP Timeout (s)
              </label>
              <input
                type="number"
                value={timeout}
                onChange={(e) => setTimeoutVal(e.target.value)}
                className="w-full bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200"
              />
            </div>

            <div>
              <label className="block text-slate-400 font-medium mb-1">
                Proxy List (-proxies)
              </label>
              <input
                type="text"
                value={proxyFile}
                onChange={(e) => setProxyFile(e.target.value)}
                className="w-full bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200"
              />
            </div>
          </div>

          <div className="pt-1 space-y-2">
            <label className="flex items-center gap-2.5 text-xs text-slate-200 cursor-pointer">
              <input
                type="checkbox"
                checked={doHypixel}
                onChange={(e) => setDoHypixel(e.target.checked)}
                className="rounded bg-[#040406] border-[#1f1f26] text-cyan-500 focus:ring-0"
              />
              Enable Automatic Hypixel Ban & Duration Verification
            </label>
            <div className="flex items-center gap-2 text-xs text-slate-500 pl-0.5">
              <span className="w-1.5 h-1.5 rounded-full bg-amber-500 shrink-0" />
              Donut Checks (donutsmp.net) — always active on every hit
            </div>
          </div>
        </div>

        <div className="bg-[#0e0e12] border border-[#1f1f26] rounded-xl p-5 space-y-4">
          <div className="flex items-center gap-2 text-xs font-bold text-slate-200 uppercase tracking-wider border-b border-[#16161c] pb-2">
            <UserCheck className="w-4 h-4 text-cyan-500" />
            Auto Name & Skin Replacement
          </div>

          <div className="space-y-4 text-xs">
            <label className="flex items-center gap-2.5 text-slate-200 cursor-pointer">
              <input
                type="checkbox"
                checked={doRename}
                onChange={(e) => setDoRename(e.target.checked)}
                className="rounded bg-[#040406] border-[#1f1f26] text-cyan-500 focus:ring-0"
              />
              Enable Auto Rename (`g00n_xxxx`) before Hypixel join
            </label>

            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 pt-1">
              <div className="sm:col-span-2">
                <label className="block text-slate-400 font-medium mb-1">
                  Local PNG Skin Path (-skin-file)
                </label>
                <div className="flex gap-2">
                  <input
                    type="text"
                    value={skinFile}
                    onChange={(e) => setSkinFile(e.target.value)}
                    placeholder="Optional path to skin.png"
                    className="flex-1 bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200"
                  />
                  <button
                    type="button"
                    className="px-3 py-1.5 bg-[#14141a] hover:bg-[#1f1f26] border border-[#1f1f26] text-slate-200 rounded-md text-xs font-medium cursor-pointer transition-colors"
                  >
                    <Image className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>

              <div>
                <label className="block text-slate-400 font-medium mb-1">
                  Skin Model Variant
                </label>
                <select
                  value={skinVariant}
                  onChange={(e) => setSkinVariant(e.target.value)}
                  className="w-full bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200"
                >
                  <option value="classic">Classic (Steve)</option>
                  <option value="slim">Slim (Alex)</option>
                </select>
              </div>
            </div>
          </div>
        </div>

        <div className="bg-[#0e0e12] border border-[#1f1f26] rounded-xl p-5 space-y-4">
          <div className="flex items-center gap-2 text-xs font-bold text-slate-200 uppercase tracking-wider border-b border-[#16161c] pb-2">
            <Send className="w-4 h-4 text-cyan-500" />
            Discord Results Integration
          </div>

          <div className="space-y-4 text-xs">
            <label className="flex items-center gap-2.5 text-slate-200 cursor-pointer">
              <input
                type="checkbox"
                checked={doUpload}
                onChange={(e) => setDoUpload(e.target.checked)}
                className="rounded bg-[#040406] border-[#1f1f26] text-cyan-500 focus:ring-0"
              />
              Enable Results Upload to Discord (ZIP)
            </label>

            <div>
              <label className="block text-slate-400 font-medium mb-1">
                Discord Webhook URL
              </label>
              <input
                type="text"
                value={discordWebhook}
                onChange={(e) => setDiscordWebhook(e.target.value)}
                placeholder="https://discord.com/api/webhooks/..."
                className="w-full bg-[#040406] border border-[#1f1f26] rounded-md px-3 py-1.5 text-slate-200 focus:outline-none focus:border-cyan-500 transition-all duration-200"
              />
            </div>
          </div>
        </div>

        <div className="flex items-center gap-3 pt-1">
          <button
            type="submit"
            className="inline-flex items-center gap-2 px-4 py-2 bg-cyan-500 hover:bg-cyan-400 text-slate-950 text-xs font-bold rounded-md shadow-lg shadow-cyan-500/10 active:scale-95 transition-all duration-150 cursor-pointer"
          >
            <Save className="w-3.5 h-3.5" />
            Save configuration
          </button>

          {saved && (
            <span className="text-xs font-bold text-emerald-400">
              ✓ Configuration saved!
            </span>
          )}
        </div>
      </form>
    </div>
  );
};
