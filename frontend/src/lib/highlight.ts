// Syntax highlighting for the diff pane, kept out of the main bundle:
// highlight.js's core loads the first time something is highlighted, and
// each language is a chunk of its own, loaded the first time a file of
// that language is shown. Only the languages listed here are built.
import type { HLJSApi, LanguageFn } from "highlight.js";
import type { DiffFile, DiffLine, Seg } from "./patch";

type Loader = () => Promise<{ default: LanguageFn }>;

const languages: Record<string, Loader> = {
  bash: () => import("highlight.js/lib/languages/bash"),
  c: () => import("highlight.js/lib/languages/c"),
  cpp: () => import("highlight.js/lib/languages/cpp"),
  csharp: () => import("highlight.js/lib/languages/csharp"),
  css: () => import("highlight.js/lib/languages/css"),
  dockerfile: () => import("highlight.js/lib/languages/dockerfile"),
  go: () => import("highlight.js/lib/languages/go"),
  ini: () => import("highlight.js/lib/languages/ini"),
  java: () => import("highlight.js/lib/languages/java"),
  javascript: () => import("highlight.js/lib/languages/javascript"),
  json: () => import("highlight.js/lib/languages/json"),
  kotlin: () => import("highlight.js/lib/languages/kotlin"),
  lua: () => import("highlight.js/lib/languages/lua"),
  makefile: () => import("highlight.js/lib/languages/makefile"),
  markdown: () => import("highlight.js/lib/languages/markdown"),
  php: () => import("highlight.js/lib/languages/php"),
  python: () => import("highlight.js/lib/languages/python"),
  ruby: () => import("highlight.js/lib/languages/ruby"),
  rust: () => import("highlight.js/lib/languages/rust"),
  scss: () => import("highlight.js/lib/languages/scss"),
  sql: () => import("highlight.js/lib/languages/sql"),
  swift: () => import("highlight.js/lib/languages/swift"),
  typescript: () => import("highlight.js/lib/languages/typescript"),
  xml: () => import("highlight.js/lib/languages/xml"),
  yaml: () => import("highlight.js/lib/languages/yaml"),
};

const byExt: Record<string, string> = {
  sh: "bash", bash: "bash", zsh: "bash", fish: "bash",
  c: "c", h: "c",
  cc: "cpp", cpp: "cpp", cxx: "cpp", hpp: "cpp", hh: "cpp",
  cs: "csharp",
  css: "css",
  go: "go",
  ini: "ini", toml: "ini", cfg: "ini", conf: "ini",
  java: "java",
  js: "javascript", mjs: "javascript", cjs: "javascript", jsx: "javascript",
  json: "json", jsonl: "json",
  kt: "kotlin", kts: "kotlin",
  lua: "lua",
  mk: "makefile",
  md: "markdown", markdown: "markdown",
  php: "php",
  py: "python", pyi: "python",
  rb: "ruby",
  rs: "rust",
  scss: "scss",
  sql: "sql",
  swift: "swift",
  ts: "typescript", tsx: "typescript", mts: "typescript", cts: "typescript",
  // Svelte and Vue files are mostly markup; xml highlights it, and the
  // script inside where javascript is loaded too.
  html: "xml", htm: "xml", xml: "xml", svg: "xml", svelte: "xml", vue: "xml", plist: "xml",
  yml: "yaml", yaml: "yaml",
};

const byName: Record<string, string> = {
  dockerfile: "dockerfile", makefile: "makefile", gnumakefile: "makefile",
  "go.mod": "go", "go.sum": "ini", "taskfile.yml": "yaml",
};

// languageFor is the language a path is highlighted as, or "" for none.
export function languageFor(path: string): string {
  const name = path.slice(path.lastIndexOf("/") + 1).toLowerCase();
  if (byName[name]) return byName[name];
  if (name.startsWith("dockerfile")) return "dockerfile";
  const dot = name.lastIndexOf(".");
  return dot > 0 ? (byExt[name.slice(dot + 1)] ?? "") : "";
}

let core: Promise<HLJSApi> | undefined;

async function load(lang: string): Promise<HLJSApi> {
  const hljs = await (core ??= import("highlight.js/lib/core").then((m) => m.default));
  // xml highlights <script> as javascript, when that is loaded.
  const need = lang === "xml" ? ["javascript", "css", "xml"] : [lang];
  for (const l of need) {
    if (!hljs.getLanguage(l)) hljs.registerLanguage(l, (await languages[l]()).default);
  }
  return hljs;
}

// A run of highlighted text; cls is highlight.js's class for it, if any.
export interface Tok {
  text: string;
  cls: string;
}

// Lines of highlighted text: what highlight.js drew, cut at each newline.
function highlightLines(hljs: HLJSApi, lang: string, code: string): Tok[][] {
  const html = hljs.highlight(code, { language: lang, ignoreIllegals: true }).value;
  // highlight.js escapes the code and adds only <span class>; a template's
  // content is inert, and is only read here, never put on the page.
  const t = document.createElement("template");
  t.innerHTML = html;
  const lines: Tok[][] = [[]];
  const walk = (n: Node, cls: string) => {
    for (const c of Array.from(n.childNodes)) {
      if (c.nodeType === Node.TEXT_NODE) {
        const parts = (c.textContent ?? "").split("\n");
        parts.forEach((p, i) => {
          if (i > 0) lines.push([]);
          if (p) lines[lines.length - 1].push({ text: p, cls });
        });
      } else if (c instanceof Element) {
        walk(c, c.getAttribute("class") ?? cls);
      }
    }
  };
  walk(t.content, "");
  return lines;
}

// Files past these are shown without highlighting: it would take too long.
const maxLines = 4000;
const maxBytes = 300 << 10;

// highlightFile highlights each hunk's old side (context and removed
// lines) and new side (context and added lines) as whole blocks, so a
// comment or string spanning lines is drawn as one, and returns each
// line's runs, hunk by hunk; null when the file is not highlighted.
export async function highlightFile(f: DiffFile): Promise<Tok[][][] | null> {
  const lang = languageFor(f.path);
  if (!lang || f.binary || f.raw.length > maxBytes) return null;
  if (f.hunks.reduce((n, h) => n + h.lines.length, 0) > maxLines) return null;
  const hljs = await load(lang);
  return f.hunks.map((h) => {
    const oldSide = h.lines.filter((l) => l.kind === "ctx" || l.kind === "del");
    const newSide = h.lines.filter((l) => l.kind === "ctx" || l.kind === "add");
    const oldToks = highlightLines(hljs, lang, oldSide.map((l) => l.text).join("\n"));
    const newToks = highlightLines(hljs, lang, newSide.map((l) => l.text).join("\n"));
    const of = new Map(oldSide.map((l, i) => [l, oldToks[i] ?? []]));
    const nf = new Map(newSide.map((l, i) => [l, newToks[i] ?? []]));
    return h.lines.map((l) => (l.kind === "del" ? of.get(l) : nf.get(l)) ?? []);
  });
}

// A piece of a drawn line: highlighted, and marked if it is a changed word.
export interface Piece {
  text: string;
  cls: string;
  changed: boolean;
}

// pieces cuts a line at both its highlighting and its changed words.
export function pieces(l: DiffLine, toks?: Tok[]): Piece[] {
  const segs: Seg[] = l.segs ?? [{ text: l.text, changed: false }];
  if (!toks?.length) return segs.map((s) => ({ text: s.text, cls: "", changed: s.changed }));
  const out: Piece[] = [];
  let si = 0, soff = 0, ti = 0, toff = 0;
  while (si < segs.length && ti < toks.length) {
    const s = segs[si], t = toks[ti];
    const n = Math.min(s.text.length - soff, t.text.length - toff);
    out.push({ text: t.text.slice(toff, toff + n), cls: t.cls, changed: s.changed });
    soff += n;
    toff += n;
    if (soff === s.text.length) { si++; soff = 0; }
    if (toff === t.text.length) { ti++; toff = 0; }
  }
  // highlight.js keeps every character, so the two end together; should
  // they not, the rest of the line is shown plain.
  for (; si < segs.length; si++, soff = 0) out.push({ text: segs[si].text.slice(soff), cls: "", changed: segs[si].changed });
  return out.filter((p) => p.text);
}
