import React from 'react';
import { render, act } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';

// ============================================================
// Mock window.PIXI (UMD globals)
// ============================================================

const mockCanvas = document.createElement('canvas');

function fakeDisplayObject(overrides: Record<string, unknown> = {}) {
  return {
    x: 0, y: 0, width: 0, height: 0,
    scale: { set: vi.fn() },
    position: { set: vi.fn() },
    anchor: { set: vi.fn() },
    getBounds: vi.fn(() => ({ x: 100, y: 100, width: 200, height: 300 })),
    toLocal: vi.fn((pt: { x: number; y: number }) => pt),
    visible: true, name: '',
    ...overrides,
  };
}

let mockModel: Record<string, unknown>;

function makeMockModel() {
  return {
    ...fakeDisplayObject({ width: 300, height: 400 }),
    expression: vi.fn(),
    motion: vi.fn(),
    focus: vi.fn(),
    hitTest: vi.fn(() => Promise.resolve(['head'])),
  };
}

function makeMockStage() {
  return {
    ...fakeDisplayObject(),
    addChild: vi.fn(),
    addChildAt: vi.fn(),
  };
}

function setupWindowPIXI() {
  const stage = makeMockStage();

  (window as any).PIXI = {
    Application: vi.fn(function (this: any) {
      this.view = mockCanvas;
      this.stage = stage;
      this.destroy = vi.fn();
    }),
    Container: vi.fn(function (this: any) {
      this.x = 0; this.y = 0; this.width = 0; this.height = 0;
      this.visible = false;
      this.addChild = vi.fn();
      this.addChildAt = vi.fn();
      this.getChildByName = vi.fn((name: string) => {
        if (name === 'text') return { x: 0, y: 0, width: 100, height: 20, anchor: { set: vi.fn() }, text: '' };
        if (name === 'bg') return { clear: vi.fn(), beginFill: vi.fn(), drawRoundedRect: vi.fn(), moveTo: vi.fn(), lineTo: vi.fn(), closePath: vi.fn(), endFill: vi.fn() };
        return null;
      });
      this.getBounds = vi.fn(() => ({ x: 100, y: 80, width: 200, height: 60 }));
    }),
    Graphics: vi.fn(function (this: any) {
      this.clear = vi.fn(); this.beginFill = vi.fn();
      this.drawRoundedRect = vi.fn(); this.moveTo = vi.fn();
      this.lineTo = vi.fn(); this.closePath = vi.fn(); this.endFill = vi.fn();
    }),
    Text: vi.fn(function (this: any) {
      this.x = 0; this.y = 0; this.width = 100; this.height = 20;
      this.text = ''; this.anchor = { set: vi.fn() };
    }),
    Point: vi.fn(function (this: any, x: number, y: number) { this.x = x; this.y = y; }),
    live2d: {
      Live2DModel: {
        from: vi.fn(() => Promise.resolve(mockModel)),
      },
    },
  };

  (window as any).PIXI.live2d.Live2DModel.from = vi.fn(() => Promise.resolve(mockModel));
}

// ============================================================
// Component under test
// ============================================================

import PetCanvas, { PetCanvasHandle } from '../PetCanvas';

// ============================================================
// Tests
// ============================================================

describe('PetCanvas', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockModel = makeMockModel();
    setupWindowPIXI();
  });

  it('renders a canvas element into the container', () => {
    const { container } = render(<PetCanvas modelPath="/test.model3.json" />);
    expect(container.querySelector('canvas')).toBeTruthy();
  });

  it('renders a container div with 100% size', () => {
    const { container } = render(<PetCanvas modelPath="/test.model3.json" />);
    const div = container.firstElementChild as HTMLElement;
    expect(div.tagName).toBe('DIV');
    expect(div.style.width).toBe('100%');
    expect(div.style.height).toBe('100%');
  });

  it('calls onReady after model loads', async () => {
    const onReady = vi.fn();
    render(<PetCanvas modelPath="/test.model3.json" onReady={onReady} />);
    await act(() => Promise.resolve());
    expect(onReady).toHaveBeenCalledTimes(1);
  });

  it('exposes playExpression and playMotion via ref', async () => {
    const ref = React.createRef<PetCanvasHandle>();
    render(<PetCanvas modelPath="/test.model3.json" ref={ref} />);
    await act(() => Promise.resolve());

    act(() => ref.current?.playExpression('happy'));
    expect(mockModel.expression).toHaveBeenCalledWith('happy');

    act(() => ref.current?.playMotion('idle', 0));
    expect(mockModel.motion).toHaveBeenCalledWith('idle', 0);
  });

  it('exposes setFocus via ref', async () => {
    const ref = React.createRef<PetCanvasHandle>();
    render(<PetCanvas modelPath="/test.model3.json" ref={ref} />);
    await act(() => Promise.resolve());

    act(() => ref.current?.setFocus(150, 200));
    expect(mockModel.focus).toHaveBeenCalledWith(150, 200, false);
  });

  it('exposes showBubble and hideBubble via ref', async () => {
    const ref = React.createRef<PetCanvasHandle>();
    render(<PetCanvas modelPath="/test.model3.json" ref={ref} />);
    await act(() => Promise.resolve());

    act(() => ref.current?.showBubble('hello'));
    act(() => ref.current?.hideBubble());
  });

  it('detects poke on short pointer tap', async () => {
    const onPoke = vi.fn();
    const { container } = render(
      <PetCanvas modelPath="/test.model3.json" onPoke={onPoke} />,
    );
    await act(() => Promise.resolve());

    const canvas = container.querySelector('canvas')!;
    vi.spyOn(canvas, 'getBoundingClientRect').mockReturnValue({
      left: 0, top: 0, width: 400, height: 500,
    } as DOMRect);

    await act(async () => {
      canvas.dispatchEvent(new PointerEvent('pointerdown', { clientX: 200, clientY: 250 }));
      canvas.dispatchEvent(new PointerEvent('pointerup', { clientX: 200, clientY: 250 }));
      await Promise.resolve();
    });

    expect(onPoke).toHaveBeenCalledTimes(1);
    expect(onPoke).toHaveBeenCalledWith(['head']);
    expect(mockModel.hitTest).toHaveBeenCalled();
  });

  it('detects drag on pointer move beyond threshold', async () => {
    const onDragStart = vi.fn();
    const onDragEnd = vi.fn();
    const { container } = render(
      <PetCanvas
        modelPath="/test.model3.json"
        onDragStart={onDragStart}
        onDragEnd={onDragEnd}
      />,
    );
    await act(() => Promise.resolve());

    const canvas = container.querySelector('canvas')!;
    vi.spyOn(canvas, 'getBoundingClientRect').mockReturnValue({
      left: 0, top: 0, width: 400, height: 500,
    } as DOMRect);

    act(() => {
      canvas.dispatchEvent(new PointerEvent('pointerdown', { clientX: 100, clientY: 200 }));
      canvas.dispatchEvent(new PointerEvent('pointermove', { clientX: 120, clientY: 220 }));
      canvas.dispatchEvent(new PointerEvent('pointerup', { clientX: 120, clientY: 220 }));
    });

    expect(onDragStart).toHaveBeenCalledTimes(1);
    expect(onDragEnd).toHaveBeenCalledTimes(1);
  });

  it('calls focus on pointer move', async () => {
    const { container } = render(<PetCanvas modelPath="/test.model3.json" />);
    await act(() => Promise.resolve());

    const canvas = container.querySelector('canvas')!;
    vi.spyOn(canvas, 'getBoundingClientRect').mockReturnValue({
      left: 0, top: 0, width: 400, height: 500,
    } as DOMRect);

    act(() => {
      canvas.dispatchEvent(new PointerEvent('pointermove', { clientX: 150, clientY: 200 }));
    });

    expect(mockModel.focus).toHaveBeenCalledWith(150, 200, false);
  });
});
