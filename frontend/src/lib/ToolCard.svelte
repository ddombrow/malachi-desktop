<script lang="ts">
  import type { Message, ToolRun } from "./session.svelte";
  import { textOf } from "./session.svelte";

  let { name, args, result, run }: {
    name: string;
    args: Record<string, any>;
    result?: Message;
    run?: ToolRun;
  } = $props();

  const icons: Record<string, string> = {
    bash: ">_", read: "👀", write: "📝", edit: "🔨", grep: "🔍", glob: "📂",
    fetch: "🌐", script: "⚙", jobs: "⏱",
  };

  let open = $state(false);

  const status = $derived(
    result ? (result.isError ? "error" : "done") : (run?.status ?? "pending"),
  );

  const summary = $derived.by(() => {
    const a = args ?? {};
    switch (name) {
      case "bash": return a.description ? `${a.description} — ${a.command}` : a.command;
      case "read": return a.path + (a.offset ? `:${a.offset}` : "");
      case "write": case "edit": return a.path;
      case "grep": return `${a.pattern}${a.path ? "  in " + a.path : ""}`;
      case "glob": return a.pattern;
      case "fetch": return a.url;
      case "script": return (a.code ?? a.script ?? "").split("\n")[0];
      default: {
        const s = JSON.stringify(a);
        return s.length > 140 ? s.slice(0, 140) + "…" : s;
      }
    }
  });

  const output = $derived(result ? textOf(result.content) : (run?.partial ?? ""));
  const patch = $derived<string>(
    (result?.details?.patch ?? run?.details?.patch ?? "") as string,
  );

  function lineClass(l: string) {
    if (l.startsWith("+++") || l.startsWith("---")) return "meta";
    if (l.startsWith("+")) return "add";
    if (l.startsWith("-")) return "del";
    if (l.startsWith("@@")) return "hunk";
    return "";
  }

  // Drop a long command/content from the args view; it is shown elsewhere.
  const argView = $derived.by(() => {
    if (name === "bash") return args?.command ?? "";
    if (name === "write") return args?.content ?? "";
    if (name === "script") return args?.code ?? args?.script ?? JSON.stringify(args, null, 2);
    if (name === "edit" && patch) return "";
    return JSON.stringify(args, null, 2);
  });
</script>

<div class="tool {status}">
  <button class="head" onclick={() => (open = !open)}>
    <span class="icon">{icons[name] ?? "🔧"}</span>
    <span class="name">{name}</span>
    <span class="summary">{summary}</span>
    <span class="state">
      {#if status === "running" || status === "pending"}<span class="spinner"></span>
      {:else if status === "error"}failed
      {/if}
    </span>
    <span class="chev">{open ? "▾" : "▸"}</span>
  </button>
  {#if open}
    <div class="body">
      {#if argView}
        <pre class="args">{argView}</pre>
      {/if}
      {#if patch}
        <pre class="diff">{#each patch.split("\n") as l}<span class={lineClass(l)}>{l + "\n"}</span>{/each}</pre>
      {/if}
      {#if output}
        <pre class="out">{output}</pre>
      {/if}
    </div>
  {:else if status === "running" && output}
    <pre class="out tail">{output.split("\n").slice(-6).join("\n")}</pre>
  {/if}
</div>

<style>
  .tool {
    border: 1px solid var(--border);
    border-radius: 8px;
    margin: 6px 0;
    background: var(--panel);
    overflow: hidden;
  }
  .tool.error { border-color: var(--err-border); }
  .head {
    all: unset;
    box-sizing: border-box;
    display: flex;
    gap: 8px;
    align-items: center;
    width: 100%;
    padding: 6px 10px;
    cursor: pointer;
    font-size: 12.5px;
  }
  .head:hover { background: var(--hover); }
  .icon { width: 1.6em; text-align: center; font-family: var(--mono); color: var(--muted); }
  .name { font-weight: 600; color: var(--accent); font-family: var(--mono); }
  .summary {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--mono);
    color: var(--fg-dim);
  }
  .state { color: var(--err); font-size: 11px; }
  .chev { color: var(--muted); }
  .body { border-top: 1px solid var(--border); }
  pre {
    margin: 0;
    padding: 8px 12px;
    font-family: var(--mono);
    font-size: 12px;
    line-height: 1.45;
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 420px;
    overflow: auto;
  }
  .args { color: var(--fg-dim); background: var(--code-bg); }
  .out { color: var(--fg); }
  .out.tail { color: var(--muted); border-top: 1px solid var(--border); max-height: 120px; }
  .diff .add { color: var(--add); background: var(--add-bg); }
  .diff .del { color: var(--del); background: var(--del-bg); }
  .diff .hunk { color: var(--accent); }
  .diff .meta { color: var(--muted); }
  .spinner {
    display: inline-block;
    width: 10px;
    height: 10px;
    border: 2px solid var(--muted);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
