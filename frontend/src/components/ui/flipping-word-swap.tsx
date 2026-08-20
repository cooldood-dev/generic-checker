import React, { useEffect, useRef, useState } from 'react';
import gsap from 'gsap';
import { cn } from '../../lib/utils';

export interface FlippingWordSwapProps {
  word1: string;
  word2: string;
  duration?: number; // in ms
  stagger?: number; // in ms
  className?: string;
  toClassName?: string;
  style?: React.CSSProperties;
  autoPlay?: boolean;
  autoPlayInterval?: number; // in ms
}

export const FlippingWordSwap: React.FC<FlippingWordSwapProps> = ({
  word1,
  word2,
  duration = 400,
  stagger = 44,
  className,
  toClassName,
  style,
  autoPlay = true,
  autoPlayInterval = 1800,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [currentWordIndex, setCurrentWordIndex] = useState(0); // 0 = word1, 1 = word2
  const timelineRef = useRef<gsap.core.Timeline | null>(null);

  const word1Chars = word1.split('');
  const word2Chars = word2.split('');

  useEffect(() => {
    if (!autoPlay) return;

    const interval = setInterval(() => {
      triggerFlip();
    }, autoPlayInterval);

    return () => clearInterval(interval);
  }, [autoPlay, autoPlayInterval, currentWordIndex]);

  const triggerFlip = () => {
    if (!containerRef.current) return;

    const currentChars = containerRef.current.querySelectorAll('.char-current');
    const nextChars = containerRef.current.querySelectorAll('.char-next');

    if (timelineRef.current) {
      timelineRef.current.kill();
    }

    const durSec = duration / 1000;
    const stagSec = stagger / 1000;

    const tl = gsap.timeline({
      onComplete: () => {
        setCurrentWordIndex((prev) => (prev === 0 ? 1 : 0));
      },
    });

    // Rotate out current chars
    tl.to(currentChars, {
      rotateX: -90,
      opacity: 0,
      duration: durSec,
      stagger: stagSec,
      ease: 'power2.inOut',
    });

    // Rotate in next chars
    tl.fromTo(
      nextChars,
      { rotateX: 90, opacity: 0 },
      {
        rotateX: 0,
        opacity: 1,
        duration: durSec,
        stagger: stagSec,
        ease: 'power2.inOut',
      },
      `-=${durSec * 0.7}`
    );

    timelineRef.current = tl;
  };

  const displayWord1 = currentWordIndex === 0 ? word1Chars : word2Chars;
  const displayWord2 = currentWordIndex === 0 ? word2Chars : word1Chars;

  return (
    <div
      ref={containerRef}
      className={cn('inline-flex relative perspective-500 cursor-pointer select-none', className)}
      style={style}
      onClick={triggerFlip}
    >
      {/* Current Active Word */}
      <span className="inline-flex">
        {displayWord1.map((char, index) => (
          <span
            key={`curr-${index}-${char}`}
            className="char-current inline-block transform-gpu origin-center"
            style={{ transformStyle: 'preserve-3d' }}
          >
            {char === ' ' ? '\u00A0' : char}
          </span>
        ))}
      </span>

      {/* Hidden Next Word position absolute */}
      <span className={cn('inline-flex absolute top-0 left-0 opacity-0', toClassName)}>
        {displayWord2.map((char, index) => (
          <span
            key={`next-${index}-${char}`}
            className="char-next inline-block transform-gpu origin-center opacity-0"
            style={{ transformStyle: 'preserve-3d', transform: 'rotateX(90deg)' }}
          >
            {char === ' ' ? '\u00A0' : char}
          </span>
        ))}
      </span>
    </div>
  );
};
