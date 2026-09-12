import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { build, transform } from "esbuild";
import { compileModule } from "svelte/compiler";

// Exercise the actual Svelte state module with a fake Go bridge. No app data,
// registry, network, or system clipboard is touched by these tests.
const bundle = await build({
  entryPoints: ["src/lib/state.svelte.ts"], bundle: true, write: false,
  format: "esm", platform: "browser", conditions: ["browser"],
  plugins: [{ name: "svelte-runes", setup(builder) {
    builder.onLoad({filter: /\.svelte\.ts$/}, async ({path}) => {
      const source = await readFile(path, "utf8");
      const js = await transform(source, {loader: "ts", target: "esnext"});
      return {contents: compileModule(js.code, {filename: path, generate: "client"}).js.code, loader: "js"};
    });
  } }],
});
let instance = 0;
async function setup(overrides = {}) {
  globalThis.document = { documentElement: { classList: {add(){}, remove(){}, toggle(){}} } };
  globalThis.window = {
    matchMedia: () => ({matches: false, addEventListener(){}, removeEventListener(){}}),
    applyTopmost: async () => {}, saveSettings: async () => {},
    ...overrides,
  };
  return import(`data:text/javascript;base64,${Buffer.from(bundle.outputFiles[0].text).toString("base64")}#${instance++}`);
}
const deferred = () => {
  let resolve, reject;
  const promise = new Promise((yes,no) => {resolve=yes;reject=no;});
  return {promise, resolve, reject};
};

test("rapid setting changes keep both patches", async () => {
  const first = deferred();
  const writes = [];
  const app = await setup({saveSettings: async settings => {
    writes.push({...settings});
    if (writes.length === 1) await first.promise;
  }});
  const theme = app.saveSettings({theme:"dark"});
  const locale = app.saveSettings({locale:"ru"});
  await Promise.resolve();
  assert.equal(writes.length,1);
  first.resolve();
  await Promise.all([theme,locale]);
  assert.equal(writes[1].theme,"dark");
  assert.equal(writes[1].locale,"ru");
  assert.equal(app.state.settings.theme,"dark");
  assert.equal(app.state.settingsPending,0);
});

test("a failed preference save is visible and does not poison later saves", async () => {
  let fail = true;
  const app = await setup({saveSettings: async () => {if(fail) throw new Error("disk full");}});
  await assert.rejects(app.saveSettings({theme:"dark"}),/disk full/);
  assert.equal(app.state.settings.theme,"system");
  assert.match(app.state.settingsError,/disk full/);
  assert.equal(app.state.settingsPending,0);
  fail=false;
  await app.saveSettings({locale:"ru"});
  assert.equal(app.state.settings.locale,"ru");
  assert.equal(app.state.settings.theme,"system");
  assert.equal(app.state.settingsError,null);
});

test("cancelled file dialogs do not report success or reload settings", async () => {
  const app = await setup({importData: async()=>false, exportData: async()=>false,
    getSettings: async()=>{throw new Error("must not reload");}});
  assert.equal(await app.importData(),false);
  assert.equal(await app.exportData(),false);
  assert.equal(app.state.settingsError,null);
});

test("import and a following preference write use the imported snapshot", async () => {
  const writes=[];
  const app=await setup({importData:async()=>true, list:async()=>[],
    getSettings:async()=>({...app.state.settings,theme:"dark"}),
    saveSettings:async value=>writes.push({...value})});
  const imported=app.importData();
  const save=app.saveSettings({locale:"ru"});
  await Promise.all([imported,save]);
  assert.equal(writes[0].theme,"dark");
  assert.equal(writes[0].locale,"ru");
});

test("late automatic update result cannot overwrite a manual check", async () => {
  const automatic=deferred();
  const info={version:"2.0.0",name:"Release",url:"https://example.com",publishedAt:""};
  const app=await setup({checkForUpdates:()=>automatic.promise,forceCheckForUpdates:async()=>info});
  const background=app.loadUpdateInfo();
  await app.forceCheckUpdateInfo();
  automatic.resolve(null);
  await background;
  assert.equal(app.state.updateInfo.version,"2.0.0");
  assert.equal(app.state.updateCheckStatus.kind,"available");
});

const signedRelease = {version:"2.1.0",name:"Release",url:"https://example.com",publishedAt:"",size:100,selfUpdate:true};
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

test("installing an update follows Go progress, then restarts", async () => {
  const install = deferred();
  let progress = {stage:"download", done:50, total:100};
  const restarts = [];
  const app = await setup({
    forceCheckForUpdates: async () => signedRelease,
    installUpdate: () => install.promise,
    updateProgress: async () => progress,
    restartApp: async () => { restarts.push(true); },
  });
  await app.forceCheckUpdateInfo();
  const run = app.installUpdate();
  assert.deepEqual(app.state.updateInstall, {kind:"downloading", done:0, total:100});
  assert.equal(app.isUpdateInstalling(), true);
  await sleep(400);
  assert.deepEqual(app.state.updateInstall, {kind:"downloading", done:50, total:100});
  progress = {stage:"verify", done:0, total:0};
  await sleep(400);
  assert.equal(app.state.updateInstall.kind, "verifying");
  install.resolve({version:"2.1.0"});
  await run;
  assert.equal(app.state.updateInstall.kind, "restarting");
  assert.equal(restarts.length, 1);
  // A second click while restarting must not start another install.
  await app.installUpdate();
  assert.equal(restarts.length, 1);
});

test("a failed update is reported, keeps the button usable and can be retried", async () => {
  let fail = true;
  const app = await setup({
    forceCheckForUpdates: async () => signedRelease,
    installUpdate: async () => { if (fail) throw new Error("signature does not match the downloaded file"); return {version:"2.1.0"}; },
    updateProgress: async () => ({stage:"", done:0, total:0}),
    restartApp: async () => {},
  });
  await app.forceCheckUpdateInfo();
  await app.installUpdate();
  assert.equal(app.state.updateInstall.kind, "failed");
  assert.equal(app.state.updateInstall.error, "signature does not match the downloaded file");
  assert.equal(app.isUpdateInstalling(), false);
  await app.forceCheckUpdateInfo();
  assert.equal(app.state.updateInstall.kind, "idle");
  fail = false;
  await app.installUpdate();
  assert.equal(app.state.updateInstall.kind, "restarting");
});

test("a release without a signed binary is never installed in-app", async () => {
  const app = await setup({
    forceCheckForUpdates: async () => ({...signedRelease, selfUpdate:false}),
    installUpdate: async () => { throw new Error("must not be called"); },
  });
  await app.forceCheckUpdateInfo();
  await app.installUpdate();
  assert.equal(app.state.updateInstall.kind, "idle");
});

// String.raw keeps the Windows separators literal without doubling them.
const DOWNLOADS = String.raw`C:\Users\x\Downloads`;
const PROGRAMS = String.raw`C:\Users\x\AppData\Local\Programs\CopyNote`;
const PORTABLE = String.raw`D:\PortableApps\CopyNote`;

const location = (over = {}) => ({
  path: DOWNLOADS + String.raw`\copynote.exe`,
  dir: DOWNLOADS,
  permanent: false,
  defaultDir: PROGRAMS,
  canRelocate: true,
  ...over,
});

const anEntry = {id: "1", label: "Email", value: "me@example.com", order: 0};

test("the move offer follows where the executable actually lives", async () => {
  const settled = await setup({
    getInstallLocation: async () => location({permanent: true, dir: PROGRAMS}),
    list: async () => [anEntry],
  });
  await settled.loadInstallLocation();
  await settled.refresh();
  assert.equal(settled.canOfferRelocate(), false, "a program folder needs no move");
  assert.equal(settled.shouldShowRelocateBanner(), false);

  const loose = await setup({
    getInstallLocation: async () => location(),
    list: async () => [anEntry],
  });
  await loose.loadInstallLocation();
  await loose.refresh();
  assert.equal(loose.canOfferRelocate(), true);
  assert.equal(loose.shouldShowRelocateBanner(), true);
});

test("the banner waits for the first entry, the settings action does not", async () => {
  const app = await setup({getInstallLocation: async () => location(), list: async () => []});
  await app.loadInstallLocation();
  await app.refresh();
  assert.equal(app.state.entries.length, 0);
  assert.equal(app.shouldShowRelocateBanner(), false, "an empty list must not open with a warning");
  assert.equal(app.canOfferRelocate(), true, "settings keeps the action from the start");

  app.state.entries = [anEntry];
  assert.equal(app.shouldShowRelocateBanner(), true, "the banner appears once there is something to protect");
});

test("an unknown executable path offers nothing", async () => {
  const app = await setup({getInstallLocation: async () => location({canRelocate: false, path: "", dir: ""})});
  await app.loadInstallLocation();
  assert.equal(app.canOfferRelocate(), false);

  const broken = await setup({getInstallLocation: async () => {throw new Error("no bridge");}});
  await broken.loadInstallLocation();
  assert.equal(broken.state.installLocation, null);
  assert.equal(broken.canOfferRelocate(), false);
});

test("remind me later hides the banner until the stored instant passes", async () => {
  const hour = 60 * 60 * 1000;
  let asked = 0;
  const app = await setup({
    getInstallLocation: async () => location(),
    list: async () => [anEntry],
    snoozeRelocatePrompt: async () => {
      asked++;
      return new Date(Date.now() + hour).toISOString();
    },
  });
  await app.loadInstallLocation();
  await app.refresh();
  assert.equal(app.shouldShowRelocateBanner(), true);

  await app.snoozeRelocatePrompt();
  assert.equal(asked, 1);
  assert.equal(app.shouldShowRelocateBanner(), false, "hidden while the snooze runs");
  assert.equal(app.state.settings.relocatePromptDismissed, false, "a snooze is not a dismissal");

  // Once the instant is in the past the offer comes back on its own.
  app.state.relocateSnoozeClicked = false;
  app.state.settings = {
    ...app.state.settings,
    relocateRemindAfter: new Date(Date.now() - hour).toISOString(),
  };
  assert.equal(app.shouldShowRelocateBanner(), true, "an expired snooze stops hiding it");
});

test("a snooze that fails to persist still hides the banner and reports the error", async () => {
  const app = await setup({
    getInstallLocation: async () => location(),
    list: async () => [anEntry],
    snoozeRelocatePrompt: async () => {throw new Error("disk is full");},
  });
  await app.loadInstallLocation();
  await app.refresh();

  await app.snoozeRelocatePrompt();
  assert.equal(app.shouldShowRelocateBanner(), false, "the click still feels like it worked");
  assert.match(app.state.settingsError, /disk is full/);
  assert.equal(app.state.settings.relocateRemindAfter, "", "nothing was stored");
});

test("a corrupted reminder instant shows the banner rather than hiding it forever", async () => {
  const app = await setup({getInstallLocation: async () => location(), list: async () => [anEntry]});
  await app.loadInstallLocation();
  await app.refresh();
  app.state.settings = {...app.state.settings, relocateRemindAfter: "not a date"};
  assert.equal(app.shouldShowRelocateBanner(), true);
});

test("dismissing hides the banner but keeps the action in settings", async () => {
  let dismissed = 0;
  const app = await setup({
    getInstallLocation: async () => location(),
    list: async () => [anEntry],
    dismissRelocatePrompt: async () => {dismissed++;},
  });
  await app.loadInstallLocation();
  await app.refresh();
  assert.equal(app.shouldShowRelocateBanner(), true, "visible before the dismissal");

  await app.dismissRelocatePrompt();
  assert.equal(dismissed, 1);
  assert.equal(app.shouldShowRelocateBanner(), false, "banner is gone");
  assert.equal(app.canOfferRelocate(), true, "settings must not lose the action");
});

test("a failed move is reported and leaves the button usable", async () => {
  const app = await setup({
    getInstallLocation: async () => location(),
    relocateApp: async () => {throw new Error("access is denied");},
  });
  await app.loadInstallLocation();
  await app.relocateApp();
  assert.equal(app.state.relocate.kind, "failed");
  assert.match(app.state.relocate.error, /access is denied/);
  assert.doesNotMatch(app.state.relocate.error, /^Error:/, "the Error: prefix is stripped");
  // Still offered, so the user can retry rather than being stuck.
  assert.equal(app.canOfferRelocate(), true);
});

test("a cancelled folder picker moves nothing", async () => {
  const moves = [];
  const app = await setup({
    getInstallLocation: async () => location(),
    pickInstallFolder: async () => "",
    relocateApp: async dir => {moves.push(dir); return dir;},
  });
  await app.loadInstallLocation();
  await app.relocateAppTo("pick a folder");
  assert.deepEqual(moves, [], "cancelling must not start a move");
  assert.equal(app.state.relocate.kind, "idle");
});

test("a chosen folder is the one the move targets", async () => {
  const moves = [];
  const app = await setup({
    getInstallLocation: async () => location(),
    pickInstallFolder: async () => PORTABLE,
    relocateApp: async dir => {moves.push(dir); return dir + String.raw`\copynote.exe`;},
  });
  await app.loadInstallLocation();
  await app.relocateAppTo("pick a folder");
  assert.deepEqual(moves, [PORTABLE]);
});

test("a second click cannot start a move while one is running", async () => {
  const gate = deferred();
  let calls = 0;
  const app = await setup({
    getInstallLocation: async () => location(),
    relocateApp: async () => {calls++; await gate.promise; return "moved";},
  });
  await app.loadInstallLocation();
  const first = app.relocateApp();
  await Promise.resolve();
  await app.relocateApp();
  assert.equal(calls, 1, "the running move is not restarted");
  gate.resolve();
  await first;
});

const twoEntries = [
  {id: "1", label: "Рабочая почта", value: "me@example.com", order: 0},
  {id: "2", label: "Личный телефон", value: "+7 999", order: 1},
];

test("Enter in the search box copies the top match", async () => {
  const copied = [];
  const app = await setup({list: async () => twoEntries, copy: async id => {copied.push(id); return null;}});
  await app.refresh();

  app.state.query = "телефон";
  assert.equal(await app.copyTopMatch(), true);
  assert.deepEqual(copied, ["2"], "the filtered first entry, not the list's first");
});

test("Enter copies nothing when the filter matches nothing", async () => {
  const copied = [];
  const app = await setup({list: async () => twoEntries, copy: async id => {copied.push(id); return null;}});
  await app.refresh();

  app.state.query = "не существует";
  assert.equal(await app.copyTopMatch(), false, "so the caller leaves the window open");
  assert.deepEqual(copied, []);
});

test("a failed clipboard write is reported and does not hide the window", async () => {
  const app = await setup({
    list: async () => twoEntries,
    copy: async () => {throw new Error("clipboard is locked");},
  });
  await app.refresh();

  assert.equal(await app.copyTopMatch(), false);
  assert.match(app.state.operationError, /clipboard is locked/);
  assert.doesNotMatch(app.state.operationError, /^Error:/, "the Error: prefix is stripped");
});

test("showing the window again clears last session's search and view", async () => {
  const app = await setup({list: async () => twoEntries});
  await app.refresh();

  app.state.query = "почта";
  app.state.operationError = "stale failure";
  app.openSettings();
  assert.equal(app.state.view, "settings");

  app.resetForShow();
  assert.equal(app.state.query, "", "a two-second copy must not start pre-filtered");
  assert.equal(app.state.view, "main", "closing from Settings must not reopen there");
  assert.equal(app.state.operationError, null);
});
