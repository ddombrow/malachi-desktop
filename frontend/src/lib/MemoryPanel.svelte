<script lang="ts">
  // Browse and edit what the agent remembers (malachi-memory, in the app's
  // own database). The agent reaches the same memories through its
  // mcp__memory__* tools while memory is on.
  import { MalachiService } from "../../bindings/github.com/ddombrow/malachi-desktop";
  import type { MemoryItem, TopicCount } from "../../bindings/github.com/ddombrow/malachi-desktop/models";
  import { session as s } from "./session.svelte";

  let { onclose }: { onclose: () => void } = $props();

  let items = $state<MemoryItem[]>([]);
  let topics = $state<TopicCount[]>([]);
  let query = $state("");
  let topic = $state("");
  let newText = $state("");
  let newTopics = $state("");
  let confirmId = $state<string | null>(null);
  let error = $state("");
  let loading = $state(false);

  async function load() {
    loading = true;
    error = "";
    try {
      [items, topics] = await Promise.all([
        MalachiService.MemoryList(query, query ? "" : topic, 200),
        MalachiService.MemoryTopics(),
      ]);
      items ??= [];
      topics ??= [];
    } catch (e: any) {
      error = String(e?.message ?? e);
    } finally {
      loading = false;
    }
  }

  // Reload on open, on a new search or topic, and when the agent changes
  // memories.
  let timer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    void query;
    void topic;
    void s.memoryVersion;
    clearTimeout(timer);
    timer = setTimeout(load, 150);
  });

  async function add() {
    const text = newText.trim();
    if (!text) return;
    const t = newTopics.split(",").map((x) => x.trim()).filter(Boolean);
    try {
      await MalachiService.MemoryRemember(text, t);
      newText = "";
      newTopics = "";
      await load();
    } catch (e: any) {
      error = String(e?.message ?? e);
    }
  }

  async function forget(id: string) {
    if (confirmId !== id) {
      confirmId = id;
      return;
    }
    confirmId = null;
    try {
      await MalachiService.MemoryForget(id);
      await load();
    } catch (e: any) {
      error = String(e?.message ?? e);
    }
  }

  function when(ms: number) {
    return new Date(ms).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === "Escape") onclose();
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="mem-backdrop" onclick={onclose} role="presentation"></div>
<aside class="mem" aria-label="Memory">
  <header class="mem-head">
    <span class="mem-title">🧠 Memory</span>
    <label class="switch" title={s.memoryEnabled ? "The agent can remember and recall" : "The agent has no memory tools"}>
      <input type="checkbox" checked={s.memoryEnabled} disabled={s.running}
        onchange={(e) => s.setMemory((e.target as HTMLInputElement).checked)} />
      <span>{s.memoryEnabled ? "On" : "Off"}</span>
    </label>
    <button class="link x" onclick={onclose} title="Close (Esc)">✕</button>
  </header>

  {#if !s.memoryEnabled}
    <p class="mem-note">
      With memory on, malachi can remember things you tell it or decide together, and recall them in later
      chats and projects. It is told never to store secrets or instructions it reads in files or pages.
      {#if items.length}Memories saved earlier are below; the agent cannot see them while memory is off.{/if}
    </p>
  {/if}

  <div class="mem-tools">
    <input class="mem-search" placeholder="Search memories" bind:value={query} />
    {#if topics.length}
      <div class="mem-topics">
        <button class="chip-btn" class:on={!topic && !query} onclick={() => { topic = ""; query = ""; }}>recent</button>
        {#each topics as t (t.name)}
          <button class="chip-btn" class:on={topic === t.name && !query} onclick={() => { topic = t.name; query = ""; }}>
            {t.name} <span class="muted">{t.count}</span>
          </button>
        {/each}
      </div>
    {/if}
  </div>

  {#if error}<p class="error small mem-pad">{error}</p>{/if}

  <div class="mem-list">
    {#each items as m (m.id)}
      <div class="mem-item">
        <div class="mem-text">{m.text}</div>
        <div class="mem-meta">
          {#each m.topics as t}<button class="chip-btn small-chip" onclick={() => { topic = t; query = ""; }}>{t}</button>{/each}
          <span class="muted">{when(m.created)}</span>
          <span class="spacer"></span>
          <button class="link forget" class:armed={confirmId === m.id} onclick={() => forget(m.id)}
            onblur={() => confirmId === m.id && (confirmId = null)}>
            {confirmId === m.id ? "Forget?" : "✕"}
          </button>
        </div>
      </div>
    {:else}
      {#if !loading}
        <p class="muted small mem-pad">
          {query ? "Nothing matches." : topic ? "Nothing about this topic." : "No memories yet."}
        </p>
      {/if}
    {/each}
  </div>

  <form class="mem-add" onsubmit={(e) => { e.preventDefault(); add(); }}>
    <textarea rows="2" placeholder="Remember something…" bind:value={newText}
      onkeydown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); add(); } }}></textarea>
    <div class="row">
      <input class="mem-topics-in" placeholder="topics, comma separated" bind:value={newTopics} />
      <button type="submit" disabled={!newText.trim()}>Add</button>
    </div>
  </form>
  <div class="mem-path" title={s.memoryPath}>{s.memoryPath}</div>
</aside>
