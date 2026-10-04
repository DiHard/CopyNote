import type { Plugin } from "vite";

/** Embed the one JS entry and stylesheet into the HTML served by Go. */
export function singleFile(): Plugin {
  return {
    name: "copynote-singlefile",
    enforce: "post",
    generateBundle(_options, bundle) {
      const html = bundle["index.html"];
      if (!html || html.type !== "asset" || typeof html.source !== "string") {
        throw new Error("Expected index.html in the frontend bundle");
      }
      let source = html.source;
      for (const [name, output] of Object.entries(bundle)) {
        if (name === "index.html") continue;
        const escapedName = name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
        let tag: RegExp;
        let replacement: string;
        if (
          output.type === "chunk" &&
          output.isEntry &&
          output.imports.length === 0 &&
          output.dynamicImports.length === 0
        ) {
          tag = new RegExp(
            `<script\\b[^>]*\\bsrc="(?:\\.?/)?${escapedName}"[^>]*>\\s*</script>`,
          );
          replacement = `<script type="module">${output.code.replace(/<\/script/gi, "<\\/script")}</script>`;
        } else if (output.type === "asset" && name.endsWith(".css")) {
          tag = new RegExp(
            `<link\\b[^>]*\\bhref="(?:\\.?/)?${escapedName}"[^>]*>`,
          );
          replacement = `<style>${String(output.source)}</style>`;
        } else {
          throw new Error(`Unexpected external frontend asset: ${name}`);
        }
        if (!tag.test(source))
          throw new Error(`Missing HTML reference to ${name}`);
        source = source.replace(tag, () => replacement);
        delete bundle[name];
      }
      html.source = source;
    },
  };
}
