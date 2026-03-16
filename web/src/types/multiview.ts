// '5-grid' extends the spec to support the 5-pane layout transition (V[H[P,P,P], H[P,P]])
export type LayoutPreset =
  | '2-horizontal'
  | '2-vertical'
  | '3-columns'
  | '3-main-side'
  | '4-grid'
  | '5-grid'
  | '6-grid'
  | 'custom';

export interface LayoutConfig {
  readonly preset: LayoutPreset;
  readonly autoSaveId?: string;
}

export interface Pane {
  readonly id: string;
  readonly sessionId: string;
}

export interface Workspace {
  readonly id: string;
  readonly name: string | null;
  readonly layout: LayoutConfig;
  readonly panes: readonly Pane[];
  readonly createdAt: string;
  readonly updatedAt: string;
}
