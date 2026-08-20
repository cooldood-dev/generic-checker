import React, { useEffect, useState } from 'react';
import { FlippingWordSwap } from '../ui/flipping-word-swap';
import { AsciiBackground } from '../ascii/AsciiBackground';
import { ShieldCheck } from 'lucide-react';

interface AuthLoadingScreenProps {
  onComplete: () => void;
}

export const AuthLoadingScreen: React.FC<AuthLoadingScreenProps> = ({ onComplete }) => {
  const [statusText, setStatusText] = useState('INITIALIZING...');
  const [isFading, setIsFading] = useState(false);

  useEffect(() => {
    const t1 = setTimeout(() => setStatusText('AUTHENTICATING...'), 700);
    const t2 = setTimeout(() => setStatusText('LOADING CONFIG...'), 1400);
    const t3 = setTimeout(() => setStatusText('READY'), 2100);

    const t4 = setTimeout(() => {
      setIsFading(true);
      setTimeout(onComplete, 500); // 500ms fade transition
    }, 2600);

    return () => {
      clearTimeout(t1);
      clearTimeout(t2);
      clearTimeout(t3);
      clearTimeout(t4);
    };
  }, [onComplete]);

  return (
    <div
      className={`fixed inset-0 z-50 flex flex-col items-center justify-center bg-[#040406] text-slate-100 transition-opacity duration-500 ${
        isFading ? 'opacity-0 pointer-events-none' : 'opacity-100'
      }`}
    >
      {/* Background ASCII Layer */}
      <AsciiBackground />

      <div className="relative z-10 flex flex-col items-center gap-6">
        {/* Brand Icon */}
        <div className="p-3 bg-cyan-500/10 border border-cyan-500/20 rounded-xl text-cyan-400 shadow-lg shadow-cyan-500/5">
          <ShieldCheck className="w-8 h-8" />
        </div>

        {/* Flipping Word Swap Animation */}
        <div className="text-4xl md:text-5xl font-extrabold tracking-wider uppercase text-slate-100">
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

        {/* Status Text Sequence */}
        <div className="flex items-center gap-2 text-xs font-semibold tracking-widest text-slate-400 uppercase">
          <span className="w-2 h-2 rounded-full bg-cyan-500 animate-pulse" />
          <span>{statusText}</span>
        </div>
      </div>
    </div>
  );
};
