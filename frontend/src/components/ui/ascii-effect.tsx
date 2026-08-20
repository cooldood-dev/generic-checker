import React, { useEffect, useRef } from 'react';
import { cn } from '../../lib/utils';

export interface AsciiEffectProps {
  imageSrc?: string;
  variant?: 'image' | 'flow' | 'glitch';
  chars?: string;
  fontSize?: number;
  scale?: number;
  fit?: 'cover' | 'contain' | 'stretch';
  brightnessBoost?: number;
  contrast?: number;
  threshold?: number;
  posterize?: number;
  dither?: 'none' | 'floyd-steinberg' | 'bayer';
  ditherStrength?: number;
  backgroundColor?: string;
  invert?: boolean;
  className?: string;
  alt?: string;
}

export const AsciiEffect: React.FC<AsciiEffectProps> = ({
  imageSrc = '/images/orange-cat.jpg',
  chars = ' .:-=+*#%@',
  fontSize = 8,
  scale = 1.15,
  fit = 'cover',
  brightnessBoost = 1.8,
  contrast = 1.25,
  threshold = 0.08,
  backgroundColor = '#080808',
  invert = false,
  className,
  alt = 'ASCII cat background',
}) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext('2d', { alpha: false });
    if (!ctx) return;

    let animationFrameId: number;
    const img = new Image();
    img.crossOrigin = 'anonymous';
    img.src = imageSrc;

    const render = () => {
      if (!canvas || !ctx || !img.complete || img.naturalWidth === 0) return;

      const rect = canvas.parentElement?.getBoundingClientRect() || {
        width: window.innerWidth,
        height: window.innerHeight,
      };

      const w = Math.floor(rect.width);
      const h = Math.floor(rect.height);

      if (canvas.width !== w || canvas.height !== h) {
        canvas.width = w;
        canvas.height = h;
      }

      ctx.fillStyle = backgroundColor;
      ctx.fillRect(0, 0, w, h);

      ctx.font = `${fontSize}px "Josefin Sans", monospace`;
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';

      // Create an offscreen canvas to sample image pixels
      const sampleCanvas = document.createElement('canvas');
      const sampleCtx = sampleCanvas.getContext('2d');
      if (!sampleCtx) return;

      const cols = Math.floor(w / (fontSize * 0.6));
      const rows = Math.floor(h / fontSize);

      sampleCanvas.width = cols;
      sampleCanvas.height = rows;

      // Draw image to sample canvas with object-fit logic
      let iw = img.naturalWidth;
      let ih = img.naturalHeight;
      let sx = 0, sy = 0, sWidth = iw, sHeight = ih;

      if (fit === 'cover') {
        const aspect = cols / rows;
        const imgAspect = iw / ih;
        if (imgAspect > aspect) {
          sWidth = ih * aspect;
          sx = (iw - sWidth) / 2;
        } else {
          sHeight = iw / aspect;
          sy = (ih - sHeight) / 2;
        }
      }

      // Apply zoom scale
      if (scale !== 1) {
        const zoomW = sWidth / scale;
        const zoomH = sHeight / scale;
        sx += (sWidth - zoomW) / 2;
        sy += (sHeight - zoomH) / 2;
        sWidth = zoomW;
        sHeight = zoomH;
      }

      sampleCtx.drawImage(img, sx, sy, sWidth, sHeight, 0, 0, cols, rows);
      const imgData = sampleCtx.getImageData(0, 0, cols, rows);
      const data = imgData.data;

      const charList = chars.split('');
      const charCount = charList.length;

      const charW = w / cols;
      const charH = h / rows;

      for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
          const idx = (r * cols + c) * 4;
          let rVal = data[idx];
          let gVal = data[idx + 1];
          let bVal = data[idx + 2];

          // Calculate luminance
          let lum = (0.299 * rVal + 0.587 * gVal + 0.114 * bVal) / 255;

          // Apply contrast & brightness boost
          lum = (lum - 0.5) * contrast + 0.5;
          lum *= brightnessBoost;

          if (lum < threshold) continue;
          if (lum > 1) lum = 1;
          if (lum < 0) lum = 0;

          if (invert) lum = 1 - lum;

          const charIndex = Math.min(
            Math.floor(lum * charCount),
            charCount - 1
          );
          const char = charList[charIndex];

          const x = c * charW + charW / 2;
          const y = r * charH + charH / 2;

          // Subtle grayscale fill
          const alpha = Math.min(0.85, lum * 0.9 + 0.1);
          ctx.fillStyle = `rgba(220, 220, 235, ${alpha})`;
          ctx.fillText(char, x, y);
        }
      }
    };

    img.onload = render;
    if (img.complete) render();

    const handleResize = () => {
      animationFrameId = requestAnimationFrame(render);
    };

    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      cancelAnimationFrame(animationFrameId);
    };
  }, [
    imageSrc,
    chars,
    fontSize,
    scale,
    fit,
    brightnessBoost,
    contrast,
    threshold,
    backgroundColor,
    invert,
  ]);

  return (
    <div
      className={cn('relative w-full h-full overflow-hidden pointer-events-none select-none', className)}
      aria-label={alt}
    >
      <canvas ref={canvasRef} className="block w-full h-full" />
    </div>
  );
};
