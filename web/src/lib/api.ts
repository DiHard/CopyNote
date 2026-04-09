// Typed wrappers around the global functions injected by Go via webview.Bind.
// Each function returns a promise that rejects with an Error whose message
// matches the Go error returned by the bound method.

import type { Entry } from "./types";

declare global {
  interface Window {
    list: () => Promise<Entry[]>;
    create: (label: string, value: string) => Promise<Entry>;
    update: (id: string, label: string, value: string) => Promise<Entry>;
    remove: (id: string) => Promise<null>;
    copy: (id: string) => Promise<Entry>;
  }
}

export const api = {
  list: (): Promise<Entry[]> => window.list(),
  create: (label: string, value: string): Promise<Entry> =>
    window.create(label, value),
  update: (id: string, label: string, value: string): Promise<Entry> =>
    window.update(id, label, value),
  remove: (id: string): Promise<null> => window.remove(id),
  copy: (id: string): Promise<Entry> => window.copy(id),
};
