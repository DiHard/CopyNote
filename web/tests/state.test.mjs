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
