import React, { memo } from 'react';
import { AsciiEffect } from '../ui/ascii-effect';

export const AsciiBackground: React.FC = memo(() => {
  return (
    <div className="fixed inset-0 z-0 pointer-events-none overflow-hidden select-none">
      {/* ASCII Canvas Layer - High contrast, high visibility */}
      <AsciiEffect
        imageSrc="/images/orange-cat.jpg"
        variant="image"
        fontSize={6}
        scale={1.05}
        fit="cover"
        brightnessBoost={1.4}
        contrast={1.3}
        threshold={0.02}
        backgroundColor="#080808"
        invert={false}
        className="w-full h-full opacity-85"
        alt="ASCII Orange Cat Background"
      />

      {/* Subtle translucent overlay to ensure full visibility while preserving text legibility */}
      <div className="absolute inset-0 bg-[#080808]/40 backdrop-blur-[1px]" />
    </div>
  );
});

AsciiBackground.displayName = 'AsciiBackground';
