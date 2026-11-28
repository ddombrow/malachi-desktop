<script lang="ts">
  // What changed in the project's repository since its last commit: a
  // summary of the files, then each file's diff under its name, whose Ask
  // puts that diff in the composer. The diff is malachi's
  // (Session.WorkingTreeDiff, run as its git tool runs git), fetched when
  // the pane opens and again after each tool, since any may write a file.
  import { untrack } from "svelte";
  import { SvelteSet } from "svelte/reactivity";
  import { MalachiService } from "../../bindings/github.com/ddombrow/malachi-desktop";
  import type { WorkingDiff } from "../../bindings/github.com/ddombrow/malachi-desktop/models";
  import { session as s } from "./session.svelte";
  import { parsePatch, type DiffFile } from "./patch";
  import { highlightFile, pieces, type Tok } from "./highlight";

  let { onclose, onask }: {
    onclose: () => void;
    onask: (path: string, raw: string) => void;
  } = $props();

  let diff = $state<WorkingDiff | null>(null);
  let error = $state("");
  let fetching = $state(false);
  let again = false; // files changed while a fetch ran: fetch once more
  let gen = 0; // a result from any but the latest fetch is dropped
  let body: HTMLElement | undefined = $state();

  // Files the user folded, by path, kept across refreshes. Large files
  // start folded; expanded holds the large ones the user opened.
  const folded = new SvelteSet<string>();
  const expanded = new SvelteSet<string>();
  const bigLines = 1500;

  const files = $derived<DiffFile[]>(diff?.patch ? parsePatch(diff.patch) : []);
  const untracked = $derived(new Set(diff?.untracked ?? []));
  const totals = $derived(files.reduce((t, f) => ({ adds: t.adds + f.adds, dels: t.dels + f.dels }), { adds: 0, dels: 0 }));

  async function fetchDiff() {
    if (fetching) {
      again = true;
      return;
    }
    fetching = true;
    const mine = ++gen;
    try {
      const d = await MalachiService.WorkingTreeDiff();
      if (mine === gen) {
        diff = d;
        error = "";
      }
    } catch (e: any) {
      if (mine === gen) error = String(e?.message ?? e);
    } finally {
      fetching = false;
      if (again) {
        again = false;
        fetchDiff();
      }
    }
  }

  // Fetch on open, after a tool or run ends, and for another session or
  // project. Bursts of tool calls are coalesced.
  let timer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    void s.filesVersion;
    void s.cwd;
    void s.state?.sessionId;
    clearTimeout(timer);
    timer = setTimeout(fetchDiff, untrack(() => diff) ? 250 : 0);
    return () => clearTimeout(timer);
  });

  // A new project starts from the top, with nothing folded.
  let lastCwd = "";
  $effect(() => {
    if (s.cwd === lastCwd) return;
    lastCwd = s.cwd;
    folded.clear();
    expanded.clear();
    body?.scrollTo({ top: 0 });
  });

  // Highlighting, done a file at a time after the diff is drawn plain, for
  // the files that are open; kept by content, so a refresh that leaves a
  // file as it was does not redo it.
  const hlCache = new Map<string, Tok[][][] | null>();
  let hlTick = $state(0); // bumped as files are highlighted
  let hlGen = 0;
  const hlKey = (f: DiffFile) => f.path + "\0" + f.raw;
  const hlFor = (f: DiffFile) => (void hlTick, hlCache.get(hlKey(f)));

  $effect(() => {
    const shown = files.filter((f) => !isFolded(f));
    const keep = new Set(files.map(hlKey));
    const mine = ++hlGen;
    (async () => {
      for (const f of shown) {
        const k = hlKey(f);
        if (hlCache.has(k)) continue;
        const r = await highlightFile(f).catch(() => null);
        hlCache.set(k, r);
        hlTick++;
        if (mine !== hlGen) return;
        await new Promise((ok) => setTimeout(ok)); // let the pane respond between files
      }
      for (const k of hlCache.keys()) if (!keep.has(k)) hlCache.delete(k);
    })();
  });

  const lineCount = (f: DiffFile) => f.hunks.reduce((n, h) => n + h.lines.length, 0);
  const isFolded = (f: DiffFile) => folded.has(f.path) || (lineCount(f) > bigLines && !expanded.has(f.path));

  function toggle(f: DiffFile) {
    if (isFolded(f)) {
      folded.delete(f.path);
      expanded.add(f.path);
    } else {
      folded.add(f.path);
      expanded.delete(f.path);
    }
  }

  function jumpTo(i: number) {
    const el = body?.querySelector<HTMLElement>(`[data-file="${i}"]`);
    if (el && body) body.scrollTo({ top: el.offsetTop - body.offsetTop, behavior: "smooth" });
  }

  // Width of the line-number columns, in characters.
  function numWidth(f: DiffFile) {
    let max = 0;
    for (const h of f.hunks) for (const l of h.lines) max = Math.max(max, l.oldNo ?? 0, l.newNo ?? 0);
    return String(max).length;
  }

  const dir = (p: string) => (p.includes("/") ? p.slice(0, p.lastIndexOf("/") + 1) : "");
  const base = (p: string) => p.slice(p.lastIndexOf("/") + 1);
  const statusTitle: Record<string, string> = { A: "added", D: "deleted", R: "renamed", M: "modified" };
</script>

<div class="dp">
  <div class="dp-head">
    <span class="dp-title">Changes</span>
    {#if files.length}
      <span class="muted small">{files.length} {files.length === 1 ? "file" : "files"}</span>
      <span class="n add">+{totals.adds}</span><span class="n del">−{totals.dels}</span>
    {/if}
    <span class="spacer"></span>
    <button class="link dp-btn" onclick={fetchDiff} title="Refresh" aria-label="Refresh">
      <span class:spin={fetching}>↻</span>
    </button>
    <button class="link dp-btn" onclick={onclose} title="Close (Ctrl+G)" aria-label="Close">✕</button>
  </div>

  {#if error}
    <div class="dp-note error">{error}</div>
  {/if}
  {#if diff?.truncated}
    <div class="dp-note warn">The diff is too large to show in full; some files are left out.</div>
  {/if}

  {#if !diff && !error}
    <div class="dp-empty muted">Loading…</div>
  {:else if diff?.noRepo}
    <div class="dp-empty muted">This project is not in a git repository, so there is nothing to compare with.</div>
  {:else if diff && !files.length && !error}
    <div class="dp-empty muted">No uncommitted changes.</div>
  {:else if files.length}
    <div class="dp-summary">
      {#each files as f, i (f.path)}
        <button class="dp-sum-row" onclick={() => jumpTo(i)} title={f.oldPath ? `${f.oldPath} → ${f.path}` : f.path}>
          <span class="st st-{f.status}" title={statusTitle[f.status]}>{untracked.has(f.path) ? "U" : f.status}</span>
          <span class="sum-path"><span class="d">{dir(f.path)}</span>{base(f.path)}</span>
          <span class="n add">{f.adds ? "+" + f.adds : ""}</span>
          <span class="n del">{f.dels ? "−" + f.dels : ""}</span>
        </button>
      {/each}
    </div>

    <div class="dp-body" bind:this={body}>
      {#each files as f, i (f.path)}
        {@const fold = isFolded(f)}
        {@const hl = fold ? undefined : hlFor(f)}
        <section class="file" data-file={i}>
          <div class="file-head">
            <button class="link fold" onclick={() => toggle(f)} aria-label={fold ? "Show diff" : "Hide diff"}>{fold ? "▸" : "▾"}</button>
            <span class="st st-{f.status}" title={statusTitle[f.status]}>{f.status}</span>
            <span class="file-path" title={f.path}>
              {#if f.oldPath}<span class="d">{f.oldPath} → </span>{/if}<span class="d">{dir(f.path)}</span>{base(f.path)}
            </span>
            {#if untracked.has(f.path)}<span class="tag">untracked</span>{/if}
            <span class="n add">{f.adds ? "+" + f.adds : ""}</span>
            <span class="n del">{f.dels ? "−" + f.dels : ""}</span>
            <button class="ask" onclick={() => onask(f.path, f.raw)} title="Put this file's diff in the message, to ask about it">Ask</button>
          </div>
          {#if fold}
            {#if !folded.has(f.path)}
              <button class="link dp-more" onclick={() => toggle(f)}>Show {lineCount(f).toLocaleString()} lines</button>
            {/if}
          {:else if f.binary}
            <div class="dp-meta">Binary file</div>
          {:else if !f.hunks.length}
            <div class="dp-meta">{f.status === "A" ? "Empty file" : f.status === "R" ? "Renamed without changes" : "No changes to its content"}</div>
          {:else}
            <div class="lines" style:--nw={numWidth(f) + "ch"}>
              {#each f.hunks as h, hi}
                {#if hi > 0}<div class="gap"></div>{/if}
                {#each h.lines as l, li}
                  {#if l.kind === "note"}
                    <div class="ln note"><span class="no"></span><span class="no"></span><span class="code">{l.text}</span></div>
                  {:else}
                    <div class="ln {l.kind}"
                      ><span class="no">{l.oldNo ?? ""}</span
                      ><span class="no">{l.newNo ?? ""}</span
                      ><span class="code">{#each pieces(l, hl?.[hi]?.[li]) as p}{#if p.changed}<mark class={p.cls}>{p.text}</mark>{:else if p.cls}<span class={p.cls}>{p.text}</span>{:else}{p.text}{/if}{:else}{" "}{/each}</span
                    ></div>
                  {/if}
                {/each}
              {/each}
            </div>
          {/if}
        </section>
      {/each}
    </div>
  {/if}
</div>

<style>
  .dp {
    display: flex; flex-direction: column; min-width: 0; min-height: 0; height: 100%;
    background: var(--side); border-left: 1px solid var(--border);
  }
  .dp-head {
    display: flex; align-items: center; gap: 8px; padding: 10px 12px;
    border-bottom: 1px solid var(--divider); font-size: 12.5px;
  }
  .dp-title { font-weight: 600; font-size: 13.5px; }
  .spacer { flex: 1; }
  .dp-btn { font-size: 14px; padding: 0 4px; color: var(--muted); }
  .dp-btn:hover { color: var(--accent); }
  .spin { display: inline-block; animation: spin 0.8s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .dp-note { padding: 8px 12px; font-size: 12.5px; border-bottom: 1px solid var(--border); }
  .dp-note.warn { background: var(--warn-bg); }
  .dp-empty { padding: 28px 16px; text-align: center; font-size: 13px; line-height: 1.5; }

  .n { font-family: var(--mono); font-size: 11.5px; white-space: nowrap; }
  .n.add { color: var(--add); }
  .n.del { color: var(--del); }
  .st {
    font-family: var(--mono); font-size: 10.5px; font-weight: 600; width: 1.4em; text-align: center;
    border-radius: 3px; flex: none;
  }
  .st-A { color: var(--add); }
  .st-D { color: var(--del); }
  .st-R, .st-M { color: var(--accent); }
  .d { color: var(--muted); }

  .dp-summary {
    max-height: 30%; overflow-y: auto; padding: 4px 6px; border-bottom: 1px solid var(--divider); flex: none;
  }
  .dp-sum-row {
    all: unset; box-sizing: border-box; display: flex; gap: 8px; align-items: center; width: 100%;
    padding: 2px 6px; border-radius: 4px; cursor: pointer; font-size: 12.5px;
  }
  .dp-sum-row:hover { background: var(--hover); }
  .sum-path { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: var(--mono); font-size: 12px; }

  .dp-body { flex: 1; overflow-y: auto; min-height: 0; position: relative; }
  .file { border-bottom: 1px solid var(--border); content-visibility: auto; contain-intrinsic-size: auto 200px; }
  .file-head {
    position: sticky; top: 0; z-index: 1; display: flex; align-items: center; gap: 8px;
    padding: 6px 10px; background: var(--panel); border-bottom: 1px solid var(--border); font-size: 12.5px;
  }
  .fold { color: var(--muted); width: 1em; padding: 0; }
  .file-path { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: var(--mono); font-size: 12px; font-weight: 600; }
  .file-path .d { font-weight: 400; }
  .tag { font-size: 10.5px; padding: 0 6px; border-radius: 999px; border: 1px solid var(--border); color: var(--muted); }
  .ask { font-size: 11.5px; padding: 1px 9px; border-radius: 999px; flex: none; }
  .ask:hover { border-color: var(--accent); color: var(--accent); }
  .dp-meta { padding: 8px 12px; font-size: 12px; color: var(--muted); }
  .dp-more { display: block; padding: 8px 12px; font-size: 12px; }

  .lines { font-family: var(--mono); font-size: 12px; line-height: 1.5; padding: 2px 0; }
  .ln { display: grid; grid-template-columns: var(--nw) var(--nw) 1fr; column-gap: 10px; padding: 0 10px 0 6px; }
  .no { color: var(--placeholder); text-align: right; user-select: none; }
  .code { white-space: pre-wrap; word-break: break-word; min-width: 0; }
  .ln.add { background: var(--add-bg); }
  .ln.del { background: var(--del-bg); }
  .ln.add .code::before, .ln.del .code::before {
    display: inline-block; width: 1.2ch; margin-left: -1.2ch; user-select: none;
  }
  .ln.add .code::before { content: "+"; color: var(--add); }
  .ln.del .code::before { content: "−"; color: var(--del); }
  .ln.add .no, .ln.del .no { color: var(--muted); }
  .ln .code { padding-left: 1.2ch; }
  .ln.note .code { color: var(--muted); font-style: italic; }
  mark { color: inherit; border-radius: 2px; }
  .ln.add mark { background: var(--add-word); }
  .ln.del mark { background: var(--del-word); }
  /* highlight.js's classes, in the app's palette (style.css). */
  .lines :global(.hljs-keyword), .lines :global(.hljs-built_in), .lines :global(.hljs-literal),
  .lines :global(.hljs-selector-tag), .lines :global(.hljs-doctag) { color: var(--hl-keyword); }
  .lines :global(.hljs-string), .lines :global(.hljs-regexp), .lines :global(.hljs-char),
  .lines :global(.hljs-template-tag), .lines :global(.hljs-addition) { color: var(--hl-string); }
  .lines :global(.hljs-number), .lines :global(.hljs-symbol), .lines :global(.hljs-bullet),
  .lines :global(.hljs-link) { color: var(--hl-number); }
  .lines :global(.hljs-comment), .lines :global(.hljs-quote), .lines :global(.hljs-meta) { color: var(--hl-comment); font-style: italic; }
  .lines :global(.hljs-title), .lines :global(.hljs-section), .lines :global(.hljs-selector-id),
  .lines :global(.hljs-selector-class) { color: var(--hl-func); }
  .lines :global(.hljs-type), .lines :global(.hljs-class), .lines :global(.hljs-tag),
  .lines :global(.hljs-name) { color: var(--hl-type); }
  .lines :global(.hljs-attr), .lines :global(.hljs-attribute), .lines :global(.hljs-variable),
  .lines :global(.hljs-template-variable), .lines :global(.hljs-params), .lines :global(.hljs-property) { color: var(--hl-attr); }
  .lines :global(.hljs-emphasis) { font-style: italic; }
  .lines :global(.hljs-strong) { font-weight: 600; }
  .gap { height: 0; border-top: 1px dashed var(--border); margin: 4px 0; }
</style>
