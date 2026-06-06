import { describe, it, expect, vi } from 'vitest';
import { ComboDetector } from '../PetCanvas';

describe('ComboDetector', () => {
  it('returns increasing count for rapid clicks within 2.5s', () => {
    const detector = new ComboDetector();
    vi.useFakeTimers();
    const now = Date.now();
    vi.setSystemTime(now);

    expect(detector.registerClick()).toBe(1);
    vi.advanceTimersByTime(200);
    expect(detector.registerClick()).toBe(2);
    vi.advanceTimersByTime(200);
    expect(detector.registerClick()).toBe(3);
    vi.advanceTimersByTime(200);
    expect(detector.registerClick()).toBe(4);
    vi.advanceTimersByTime(200);
    expect(detector.registerClick()).toBe(5);

    vi.useRealTimers();
  });

  it('expires old clicks after 2.5s', () => {
    const detector = new ComboDetector();
    vi.useFakeTimers();
    const now = Date.now();
    vi.setSystemTime(now);

    expect(detector.registerClick()).toBe(1);
    vi.advanceTimersByTime(200);
    expect(detector.registerClick()).toBe(2);
    vi.advanceTimersByTime(200);
    expect(detector.registerClick()).toBe(3);

    // Advance past 2.5s — all old clicks should expire
    vi.advanceTimersByTime(2600);

    // Fresh click should be count=1
    expect(detector.registerClick()).toBe(1);

    vi.useRealTimers();
  });

  it('returns 1 for first click', () => {
    const detector = new ComboDetector();
    expect(detector.registerClick()).toBe(1);
  });

  it('returns 2 for two clicks within window', () => {
    const detector = new ComboDetector();
    vi.useFakeTimers();
    const now = Date.now();
    vi.setSystemTime(now);

    expect(detector.registerClick()).toBe(1);
    vi.advanceTimersByTime(500);
    expect(detector.registerClick()).toBe(2);

    vi.useRealTimers();
  });
});
