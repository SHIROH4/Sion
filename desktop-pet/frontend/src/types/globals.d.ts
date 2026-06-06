// PIXI loaded as UMD script before app bundle. pixi-live2d-display attaches to window.PIXI.live2d.
import type * as PIXI_NS from 'pixi.js'

declare global {
  interface Window {
    PIXI: typeof PIXI_NS & {
      live2d: {
        Live2DModel: typeof import('pixi-live2d-display').Live2DModel
      }
    }
    go?: {
      main?: {
        App?: {
          Poke(areas: string[]): Promise<void>
          ResizeWindow(w: number, h: number): Promise<void>
          DragWindow(): Promise<void>
          SendMessage(text: string): Promise<void>
          OpenSettings(): Promise<void>
        }
        SettingsApp?: {
          OpenPet(): Promise<void>
        }
      }
    }
  }
}

export {}
