import React, { useEffect, useState } from 'react';
import { FlippingWordSwap } from '../ui/flipping-word-swap';
import { AsciiBackground } from '../ascii/AsciiBackground';
import { ShieldCheck, Loader2 } from 'lucide-react';

interface LicenseScreenProps {
  onComplete: () => void;
}

export const LicenseScreen: React.FC<LicenseScreenProps> = ({ onComplete }) => {
  const [license, setLicense] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [statusText, setStatusText] = useState('VALIDATING SESSION...');
  const [loading, setLoading] = useState(true);
  const [isFading, setIsFading] = useState(false);

  useEffect(() => {
    // Auth check bypassed for testing
    const checkState = async () => {
      setStatusText('AUTHENTICATED');
      setTimeout(() => {
        onComplete();
      }, 500);
    };

    // Wails backend takes a brief moment to initialize
    setTimeout(checkState, 800);
  }, [onComplete]);

  // Also listen for revocation events
  useEffect(() => {
    const handleRevoked = (errMessage: string) => {
      setLoading(false);
      setError(errMessage || "License revoked.");
    };
    // @ts-ignore
    if (window.runtime) {
      // @ts-ignore
      window.runtime.EventsOn("license:revoked", handleRevoked);
      return () => {
        // @ts-ignore
        window.runtime.EventsOff("license:revoked");
      };
    }
  }, []);

  const handleActivate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!license.trim()) return;

    setError(null);
    setLoading(true);
    setStatusText('VERIFYING KEY...');

    try {
      // @ts-ignore
      await window.go.main.App.ActivateLicense(license.trim());
      // Successful activation
      setStatusText('ACTIVATED');
      setTimeout(() => {
        setIsFading(true);
        setTimeout(onComplete, 500);
      }, 600);
    } catch (err: any) {
      // Map common error reasons to user-friendly messages
      let msg = err.toString();
      if (msg.includes("hwid_mismatch")) msg = "License is bound to another device.";
      else if (msg.includes("invalid_license")) msg = "Invalid license key.";
      else if (msg.includes("license_expired")) msg = "License has expired.";
      else if (msg.includes("network_error")) msg = "Network error. Check connection.";
      else if (msg.includes("license_revoked")) msg = "License was revoked by admin.";

      setError(msg);
      setLoading(false);
      setStatusText('ACTIVATION FAILED');
    }
  };

  return (
    <div
      className={`fixed inset-0 z-50 flex flex-col items-center justify-center bg-[#040406] text-slate-100 transition-opacity duration-500 ${
        isFading ? 'opacity-0 pointer-events-none' : 'opacity-100'
      }`}
    >
      <AsciiBackground />

      <div className="relative z-10 flex flex-col items-center gap-8 bg-[#0a0a0f]/80 p-10 rounded-2xl border border-white/5 backdrop-blur-md shadow-2xl w-full max-w-md">
        <div className="p-4 bg-cyan-500/10 border border-cyan-500/20 rounded-2xl text-cyan-400 shadow-[0_0_30px_rgba(6,182,212,0.15)]">
          <ShieldCheck className="w-10 h-10" />
        </div>

        <div className="text-3xl md:text-4xl font-extrabold tracking-wider uppercase text-slate-100">
          <FlippingWordSwap
            word1="Generic"
            word2="Checker"
            duration={450}
            stagger={40}
            autoPlay={true}
            autoPlayInterval={1100}
            className="text-slate-100"
            toClassName="text-cyan-400"
          />
        </div>

        <div className="w-full flex flex-col gap-4">
          <form onSubmit={handleActivate} className="flex flex-col gap-4 w-full">
            <div className="relative group">
              <input
                type="text"
                value={license}
                onChange={(e) => setLicense(e.target.value)}
                disabled={loading}
                placeholder="XXXXX-XXXXX-XXXXX-XXXXX"
                className="w-full bg-black/50 border border-white/10 rounded-lg px-4 py-3 text-center text-sm font-mono tracking-widest outline-none focus:border-cyan-500/50 transition-colors disabled:opacity-50"
              />
            </div>

            <button
              type="submit"
              disabled={loading || !license.trim()}
              className="w-full bg-cyan-500 hover:bg-cyan-400 text-black font-bold py-3 px-4 rounded-lg flex items-center justify-center gap-2 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {loading && <Loader2 className="w-4 h-4 animate-spin" />}
              {loading ? 'PROCESSING...' : 'ACTIVATE'}
            </button>
          </form>

          {error && (
            <div className="text-red-400 text-xs font-mono text-center bg-red-400/10 py-2 px-3 rounded border border-red-400/20">
              {error}
            </div>
          )}

          <div className="flex items-center justify-center gap-2 text-xs font-semibold tracking-widest text-slate-400 uppercase mt-4">
            <span className={`w-2 h-2 rounded-full ${loading ? 'bg-cyan-500 animate-pulse' : 'bg-slate-500'}`} />
            <span>{statusText}</span>
          </div>
        </div>
      </div>
    </div>
  );
};
