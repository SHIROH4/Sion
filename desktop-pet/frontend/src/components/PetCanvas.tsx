import { useRef, useEffect, useImperativeHandle, forwardRef } from 'react';

// PIXI + live2d loaded as UMD <script> tags in index.html:
//   1. pixi.min.js          → window.PIXI
//   2. live2dcubismcore.min.js → window.Live2DCubismCore
//   3. pixi-live2d-display.js  → window.PIXI.live2d.Live2DModel

export interface PetCanvasHandle {
  playExpression(id: string): void;
  playMotion(group: string, index: number): void;
  showBubble(text: string): void;
  showBubbleLive(text: string): void;
  hideBubble(): void;
  setFocus(x: number, y: number, instant?: boolean): void;
}

export interface PetCanvasProps {
  modelPath: string;
  onReady?: () => void;
  onPoke?: (areas: string[]) => void;
  onDragStart?: () => void;
  onDragEnd?: () => void;
}

const POKE_THRESHOLD_MS = 250;
const DRAG_THRESHOLD_PX = 4;
const EXPRESSION_RESET_MS = 3000;
const BUBBLE_HIDE_MS = 6000;
const GAZE_RESET_MS = 3000;

export class ComboDetector {
  private clicks: number[] = [];

  registerClick(): number {
    const now = Date.now();
    this.clicks = this.clicks.filter((t) => now - t < 2500);
    this.clicks.push(now);
    return this.clicks.length;
  }
}

const PetCanvas = forwardRef<PetCanvasHandle, PetCanvasProps>(
  ({ modelPath, onReady, onPoke, onDragStart, onDragEnd }, ref) => {
    const containerRef = useRef<HTMLDivElement>(null);
    const appRef = useRef<any>(null);
    const modelRef = useRef<any>(null);
    const bubbleElRef = useRef<HTMLDivElement>(null);
    const bubbleQueueRef = useRef<string[]>([]);
    const bubbleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const dragRef = useRef({ active: false, startX: 0, startY: 0, startTime: 0 });
    const propsRef = useRef({ onReady, onPoke, onDragStart, onDragEnd });
    propsRef.current = { onReady, onPoke, onDragStart, onDragEnd };

    const timerRef = useRef({
      lastExpressionAt: 0,
      lastBubbleAt: 0,
      lastGazeAt: 0,
      gazeCenterX: 0,
      gazeCenterY: 0,
      combo: new ComboDetector(),
    });

    // Split text into ~2-line chunks, breaking at semantic boundaries.
    const splitBubbleText = (text: string): string[] => {
      const MAX = 28;
      if (text.length <= MAX) return text ? [text] : [];

      const chunks: string[] = [];
      let remaining = text;
      while (remaining.length > 0) {
        if (remaining.length <= MAX) {
          chunks.push(remaining);
          break;
        }
        // Prefer sentence boundaries (。！？\n), then clause boundaries (，、), then fallback to MAX.
        let cut = -1;
        for (const sep of ['。', '！', '？', '\n', '，', '、']) {
          const idx = remaining.lastIndexOf(sep, MAX);
          if (idx > MAX * 0.5) { cut = idx + 1; break; }
        }
        if (cut < 0) cut = MAX;
        chunks.push(remaining.slice(0, cut).trim());
        remaining = remaining.slice(cut).trim();
      }
      return chunks.filter((c) => c.length > 0);
    };

    const showNextBubble = () => {
      if (bubbleTimerRef.current) {
        clearTimeout(bubbleTimerRef.current);
        bubbleTimerRef.current = null;
      }
      const q = bubbleQueueRef.current;
      if (q.length === 0) {
        bubbleElRef.current?.classList.remove('show');
        return;
      }
      const el = bubbleElRef.current;
      if (el) {
        el.textContent = q[0];
        el.classList.add('show');
      }
      q.shift();
      timerRef.current.lastBubbleAt = Date.now();
      bubbleTimerRef.current = setTimeout(showNextBubble, 3000);
    };

    const stopBubbleQueue = () => {
      bubbleQueueRef.current = [];
      if (bubbleTimerRef.current) {
        clearTimeout(bubbleTimerRef.current);
        bubbleTimerRef.current = null;
      }
    };

    useImperativeHandle(ref, () => ({
      playExpression(id: string) {
        timerRef.current.lastExpressionAt = Date.now();
        modelRef.current?.expression?.(id);
      },
      playMotion(group: string, index: number) { modelRef.current?.motion?.(group, index); },
      showBubble(t: string) {
        timerRef.current.lastBubbleAt = Date.now();
        stopBubbleQueue();
        if (!t) return;
        bubbleQueueRef.current = splitBubbleText(t);
        showNextBubble();
      },
      showBubbleLive(t: string) {
        timerRef.current.lastBubbleAt = Date.now();
        stopBubbleQueue();
        const el = bubbleElRef.current;
        if (el && t) {
          el.textContent = t;
          el.classList.add('show');
        }
      },
      hideBubble() {
        timerRef.current.lastBubbleAt = 0;
        stopBubbleQueue();
        bubbleElRef.current?.classList.remove('show');
      },
      setFocus(x: number, y: number, instant?: boolean) { modelRef.current?.focus?.(x, y, instant ?? false); },
    }), []);

    useEffect(() => {
      const container = containerRef.current;
      if (!container) return;

      const P = window.PIXI;
      const LM = P.live2d.Live2DModel;
      let destroyed = false;

      const app = new P.Application({
        backgroundAlpha: 0,
        transparent: true,
        useContextAlpha: 'notMultiplied',
        resizeTo: container,
        antialias: true,
        resolution: window.devicePixelRatio || 1,
        autoDensity: true,
      });
      appRef.current = app;
      container.appendChild(app.view as HTMLCanvasElement);

      const updateGazeTimer = () => { timerRef.current.lastGazeAt = Date.now(); };

      const getCanvasPos = (e: PointerEvent) => {
        const r = (app.view as HTMLCanvasElement).getBoundingClientRect();
        return { x: e.clientX - r.left, y: e.clientY - r.top };
      };
      const onPointerDown = (e: PointerEvent) => {
        const { x, y } = getCanvasPos(e);
        dragRef.current = { active: false, startX: x, startY: y, startTime: Date.now() };
        try { window.go?.main?.App?.DragWindow(); } catch {}
      };
      const onPointerMove = (e: PointerEvent) => {
        const { x, y } = getCanvasPos(e);
        const d = dragRef.current;
        modelRef.current?.focus?.(x, y, false);
        updateGazeTimer();
        if (!d.active && (Math.abs(x - d.startX) > DRAG_THRESHOLD_PX || Math.abs(y - d.startY) > DRAG_THRESHOLD_PX)) {
          d.active = true;
          propsRef.current.onDragStart?.();
        }
      };
      const onPointerUp = async (e: PointerEvent) => {
        const d = dragRef.current;
        if (d.active) { propsRef.current.onDragEnd?.(); return; }
        if (Date.now() - d.startTime < POKE_THRESHOLD_MS) {
          const combo = timerRef.current.combo;
          const count = combo.registerClick();
          const { x, y } = getCanvasPos(e);
          const m = modelRef.current;
          if (m) {
            const areas = await m.hitTest?.(x, y);
            if (areas?.length) {
              propsRef.current.onPoke?.(areas);
              if (count >= 10) {
                m.motion?.('special_01', 0);
              } else if (count >= 5) {
                m.expression?.('exp_06');
                timerRef.current.lastExpressionAt = Date.now();
              } else if (count >= 3) {
                m.expression?.('exp_04');
                timerRef.current.lastExpressionAt = Date.now();
              }
            }
          }
        }
      };
      const canvas = app.view as HTMLCanvasElement;
      canvas.addEventListener('pointerdown', onPointerDown);
      canvas.addEventListener('pointermove', onPointerMove);
      canvas.addEventListener('pointerup', onPointerUp);

      const tick = setInterval(() => {
        if (destroyed) return;
        const now = Date.now();
        const t = timerRef.current;
        if (t.lastExpressionAt > 0 && now - t.lastExpressionAt > EXPRESSION_RESET_MS) {
          t.lastExpressionAt = 0;
          modelRef.current?.expression?.('exp_01');
        }
        if (t.lastBubbleAt > 0 && now - t.lastBubbleAt > BUBBLE_HIDE_MS) {
          t.lastBubbleAt = 0;
          stopBubbleQueue();
          bubbleElRef.current?.classList.remove('show');
        }
        if (t.lastGazeAt > 0 && now - t.lastGazeAt > GAZE_RESET_MS) {
          t.lastGazeAt = 0;
          modelRef.current?.focus?.(t.gazeCenterX, t.gazeCenterY, true);
        }
      }, 200);

      // Re-center model on window resize
      const ro = new ResizeObserver(() => {
        const m = modelRef.current;
        if (!m) return;
        const cw = container.clientWidth;
        const ch = container.clientHeight;
        m.position?.set(cw / 2, ch / 2);
        timerRef.current.gazeCenterX = cw / 2;
        timerRef.current.gazeCenterY = ch / 2;
      });
      ro.observe(container);

      LM.from(modelPath)
        .then((model: any) => {
          if (destroyed) return;
          modelRef.current = model;
          const cw = container.clientWidth;
          const ch = container.clientHeight;

          // Scale model to 90% of window height — like Alife.
          const s = (ch * 0.9) / model.height;
          model.scale.set(s);

          // Focus animation easing — like Alife.
          const ctrl = model.internalModel?.focusController;
          if (ctrl) {
            ctrl.acceleration = 0.04;
            ctrl.deceleration = 0.08;
          }

          model.anchor.set(0.5, 0.5);
          model.position.set(cw / 2, ch / 2);
          app.stage.addChildAt(model, 0);

          const b = model.getBounds();
          timerRef.current.gazeCenterX = cw / 2;
          timerRef.current.gazeCenterY = ch / 2;

          // Resize window to tightly fit model — eliminates empty click-blocking space.
          const margin = 20;
          const newW = Math.ceil(b.width + margin * 2);
          const newH = Math.ceil(b.height + margin);
          try { window.go?.main?.App?.ResizeWindow(newW, newH); } catch {}

          propsRef.current.onReady?.();
        })
        .catch((err: any) => {
          console.error('[PetCanvas] Failed to load model:', err);
        });

      const wr = (window as any).runtime;
      if (wr?.EventsOn) {
        wr.EventsOn('mouse:move', (pos: any) => {
          if (!destroyed && modelRef.current) {
            modelRef.current.focus?.(pos.x, pos.y, false);
            updateGazeTimer();
          }
        });
        wr.EventsOn('pet:expression', (id: string) => {
          if (!destroyed && modelRef.current) {
            timerRef.current.lastExpressionAt = Date.now();
            modelRef.current.expression?.(id);
          }
        });
        wr.EventsOn('pet:motion', (data: any) => {
          if (!destroyed && modelRef.current) {
            modelRef.current.motion?.(data.group, data.index);
          }
        });
        wr.EventsOn('pet:bubble', (data: any) => {
          if (!destroyed) {
            timerRef.current.lastBubbleAt = Date.now();
            stopBubbleQueue();
            if (data.text) {
              bubbleQueueRef.current = splitBubbleText(data.text);
              showNextBubble();
            }
          }
        });
        wr.EventsOn('pet:hide_bubble', () => {
          bubbleElRef.current?.classList.remove('show');
          timerRef.current.lastBubbleAt = 0;
        });
      }

      return () => {
        destroyed = true;
        stopBubbleQueue();
        ro.disconnect();
        clearInterval(tick);
        canvas.removeEventListener('pointerdown', onPointerDown);
        canvas.removeEventListener('pointermove', onPointerMove);
        canvas.removeEventListener('pointerup', onPointerUp);
        app.destroy(true, { children: true, texture: true });
        appRef.current = null;
        modelRef.current = null;
      };
    }, [modelPath]);

    return (
      <div ref={containerRef} style={{ width: '100%', height: '100%', overflow: 'hidden', background: 'transparent', position: 'relative' }}>
        <div
          ref={bubbleElRef}
          className="pet-bubble"
        />
      </div>
    );
  },
);

PetCanvas.displayName = 'PetCanvas';
export default PetCanvas;
