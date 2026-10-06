import type { Plugin } from "vite";

/**
 * Makes JavaScript safe to sit inside a <script> element. The HTML parser
 * knows nothing about string literals: "</script" ends the element wherever
 * it stands, and "<!--" followed further on by "<script" makes it take the
 * real closing tag for text — the rest of the page then becomes script and
 * the window opens empty. In a module neither can occur outside a string,
 * regular expression or template literal, and there \x3C is another way to
 * write "<".
 */
function inlineScript(code: string): string {
  return code.replace(/<(?=\/script|!--)/gi, "\\x3C");
}

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
          replacement = `<script type="module">${inlineScript(output.code)}</script>`;
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
