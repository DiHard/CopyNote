// Fake Go bridge for the UX harness. Loaded before the bundle's module
// script, so the real UI boots against this instead of webview.Bind.
//
// Every global here mirrors one entry of the `declare global` block in
// web/src/lib/api.ts — that block is the authoritative list. A binding the
// UI calls but this file forgets fails as "undefined is not a function",
// usually with no visible symptom, so keep the two in step.
(function () {
  const flags = new URLSearchParams(location.search);

  // Bindings resolve asynchronously in the real app; a short delay keeps
  // ordering bugs visible instead of hiding them behind instant promises.
  const reply = (value) => new Promise((r) => setTimeout(() => r(value), 40));

  let seq = 0;
  const uid = () => "id-" + ++seq;

  const SAMPLE = [
    ["Work email", "maxim.dietrich@company-name.example.com"],
    ["Personal phone", "+7 999 123-45-67"],
    ["Tax number", "770912345678"],
    ["Delivery address", "125009, Moscow, Tverskaya st. 1, apt. 42"],
    ["SSH key (prod)", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIH8k2mQz9Xw deploy@prod"],
    // Multi-line on purpose: the card truncates it to one line.
    ["Reply template", "Hello!\nThanks for reaching out — we will get back to you\nwithin one working day.\n\nSupport"],
    ["Company details", "Gamma LLC, tax 7709123456, account 40702810900000012345"],
    ["git config", "git config --global user.email me@example.com"],
  ];

  function seed() {
    if (flags.has("empty")) return [];
    const rows = flags.has("many")
      ? Array.from({ length: 30 }, (_, i) => ["Entry number " + (i + 1), "value-" + (i + 1)])
      : SAMPLE;
    return rows.map(([label, value], order) => ({ id: uid(), label, value, order }));
  }

  let entries = seed();
  const copy = () => entries.map((e) => ({ ...e }));

  let settings = {
    autorun: true,
    theme: "system",
    locale: "system",
    topmost: true,
    disableUpdateCheck: false,
    disableAutoHide: false,
    relocatePromptDismissed: false,
    relocateRemindAfter: "",
    hotkey: "",
    lastSeenUpdateVersion: "",
  };

  // Spies, for asserting that a binding was called at all.
  const calls = { resizeWindow: [], hide: 0, copy: [], topmost: [], autoHide: [], hotkey: [], menu: [] };
  window.__harness = { calls, get entries() { return copy(); } };

  // ── Entries ──────────────────────────────────────────────────────
  window.list = () => reply(copy());
  window.create = (label, value) => {
    entries = entries.map((e) => ({ ...e, order: e.order + 1 }));
    const created = { id: uid(), label, value, order: 0 };
    entries.unshift(created);
    return reply({ ...created });
  };
  window.update = (id, label, value) => {
    const e = entries.find((x) => x.id === id);
    Object.assign(e, { label, value });
    return reply({ ...e });
  };
  window.remove = (id) => {
    entries = entries.filter((e) => e.id !== id).map((e, i) => ({ ...e, order: i }));
    return reply(null);
  };
  window.reorder = (ids) => {
    entries = ids.map((id, i) => ({ ...entries.find((e) => e.id === id), order: i }));
    return reply(null);
  };
  window.copy = (id) => {
    calls.copy.push(id);
    return reply(entries.find((e) => e.id === id) ?? null);
  };

  // ── Window ───────────────────────────────────────────────────────
  window.hide = () => {
    calls.hide++;
    console.log("[harness] hide()");
    return reply();
  };
  // The title is the only visible channel for the height the UI asks for,
  // which makes the auto-resize logic observable from a screenshot.
  window.resizeWindow = (h) => {
    calls.resizeWindow.push(h);
    document.title = "CopyNote — h=" + h;
    return reply();
  };
  window.applyTopmost = (v) => { calls.topmost.push(v); return reply(); };
  window.applyAutoHide = (v) => { calls.autoHide.push(v); return reply(); };
  // ?hotkeytaken makes Windows refuse the combination, so the error path in
  // Settings can be seen without actually occupying a shortcut.
  window.applyHotkey = (spec) => {
    calls.hotkey.push(spec);
    return flags.has("hotkeytaken")
      ? Promise.reject(new Error("register " + spec + ": Hot key is already registered."))
      : reply();
  };

  // ── Settings and data ────────────────────────────────────────────
  window.getSettings = () => reply({ ...settings });
  window.saveSettings = (next) => { settings = { ...next }; return reply(); };
  window.exportData = () => reply(true);
  window.importData = () => reply(true);
  window.openExternal = (url) => { console.log("[harness] openExternal", url); return reply(); };
  window.openAppFolder = () => reply();
  window.notifyReady = () => reply();
  window.getVersion = () => reply("0.0.0-harness");

  // ── Updates ──────────────────────────────────────────────────────
  // ?update offers a signed release so the in-app install flow can be driven.
  const release = flags.has("update")
    ? {
        version: "9.9.9",
        name: "Harness release",
        url: "https://example.com/release",
        publishedAt: "2026-01-01T00:00:00Z",
        size: 7_000_000,
        selfUpdate: true,
      }
    : null;
  window.checkForUpdates = () => reply(release);
  window.forceCheckForUpdates = () => reply(release);
  window.installUpdate = () => reply({ version: "9.9.9" });
  window.updateProgress = () => reply({ stage: "", done: 0, total: 0 });
  window.restartApp = () => reply();

  // ── Install location ─────────────────────────────────────────────
  // Default is a program folder, so the relocate banner stays out of the
  // way; ?downloads puts the exe somewhere temporary to bring it back.
  const inDownloads = flags.has("downloads");
  window.getInstallLocation = () =>
    reply({
      path: inDownloads
        ? String.raw`C:\Users\Test\Downloads\copynote.exe`
        : String.raw`C:\Users\Test\AppData\Local\Programs\CopyNote\copynote.exe`,
      dir: inDownloads
        ? String.raw`C:\Users\Test\Downloads`
        : String.raw`C:\Users\Test\AppData\Local\Programs\CopyNote`,
      permanent: !inDownloads,
      defaultDir: String.raw`C:\Users\Test\AppData\Local\Programs\CopyNote`,
      canRelocate: true,
    });
  window.pickInstallFolder = () => reply("");
  window.relocateApp = (dir) => reply(dir);
  window.dismissRelocatePrompt = () => {
    settings = { ...settings, relocatePromptDismissed: true };
    return reply();
  };
  window.snoozeRelocatePrompt = () => {
    const until = new Date(Date.now() + 7 * 24 * 3600 * 1000).toISOString();
    settings = { ...settings, relocateRemindAfter: until };
    return reply(until);
  };

  // ── Entry context menu ───────────────────────────────────────────
  // Go draws this menu as a window of its own, so nothing opens here: the
  // request is recorded, and __harness.pickMenu("edit") answers it
  // (pickMenu("") dismisses it).
  window.showEntryMenu = (req) => {
    calls.menu.push(req);
    return reply();
  };
  window.__harness.pickMenu = (id) => {
    const last = calls.menu[calls.menu.length - 1];
    if (last) window.__entryMenuClosed?.(last.token, id);
  };
})();
