<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import { Browser } from "@wailsio/runtime";
  import { session, textOf, type Message, type Block } from "./lib/session.svelte";
  import { renderMarkdown } from "./lib/markdown";
  import ToolCard from "./lib/ToolCard.svelte";
  import { MalachiService } from "../bindings/github.com/ddombrow/malachi-desktop";

  const s = session;

  let input = $state("");
  let scroller: HTMLElement | undefined = $state();
  let textarea: HTMLTextAreaElement | undefined = $state();
  let follow = $state(true); // keep the view pinned to new output
  let unseen = $state(false); // output arrived while scrolled up
  let jumping = false; // a smooth scroll to the bottom is under way
  let showLog = $state(false);

  onMount(async () => {
    await s.init();
    textarea?.focus();
  });

  // Tool results by call id, so cards can show their output.
  const results = $derived.by(() => {
    const m: Record<string, Message> = {};
    for (const msg of s.messages) if (msg.role === "toolResult" && msg.toolCallId) m[msg.toolCallId] = msg;
    return m;
  });

  // Follow new output unless the user has scrolled up; then note it, so the
  // jump button can say there is something new.
  $effect(() => {
    void s.messages.length;
    void s.messages[s.messages.length - 1];
    void s.toolRuns;
    if (untrack(() => follow)) tick().then(() => scroller && (scroller.scrollTop = scroller.scrollHeight));
    else unseen = true;
  });

  // Sending a message or opening something re-enables follow; nothing is
  // unseen then. A different session starts at its latest output.
  $effect(() => {
    if (follow) unseen = false;
  });
  $effect(() => {
    void s.state?.sessionId;
    void s.cwd;
    follow = true;
    tick().then(() => scroller && (scroller.scrollTop = scroller.scrollHeight));
  });

  function onScroll() {
    if (!scroller) return;
    const near = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 40;
    // While jumping, the smooth scroll passes through "not at the bottom".
    if (jumping && !near) return;
    jumping = false;
    follow = near;
    if (near) unseen = false;
  }

  function jumpToBottom() {
    if (!scroller) return;
    follow = true;
    unseen = false;
    jumping = true;
    setTimeout(() => (jumping = false), 1000); // in case the user scrolls away mid-jump
    scroller.scrollTo({ top: scroller.scrollHeight, behavior: "smooth" });
  }

  async function focusInput() {
    follow = true;
    await tick();
    textarea?.focus();
  }

  // Open a project, continuing its latest session.
  async function openProject(path?: string) {
    const d = path ?? (await MalachiService.PickDirectory(s.isChat ? "" : s.cwd));
    if (!d) return;
    await s.openProject(d, { continue: true });
    await focusInput();
  }

  const shortPath = (p: string) => p.replace(/^\/(home|Users)\/[^/]+/, "~");
  // Recent projects, with the open one included even before malachi has
  // saved a session for it.
  const projects = $derived.by(() => {
    const list = [...s.recent];
    if (!s.isChat && s.cwd && !list.some((r) => r.path === s.cwd)) {
      list.unshift({ path: s.cwd, name: s.cwd.split("/").pop() || s.cwd, lastUsed: 0, sessions: 0 });
    }
    return list;
  });
  const projectName = $derived(s.cwd.split("/").pop() || s.cwd);

  async function closeProject() {
    await s.openChat();
    await focusInput();
  }

  async function slash(text: string): Promise<boolean> {
    const [cmd, ...rest] = text.slice(1).split(/\s+/);
    const arg = rest.join(" ").trim();
    switch (cmd) {
      case "new": await s.newSession(); return true;
      case "compact": s.compact(arg); return true;
      case "network": await s.setNetwork(arg !== "off"); return true;
      case "allow-read": await s.allowRoot("read", arg); return true;
      case "allow-write": await s.allowRoot("write", arg); return true;
      case "thinking": await s.setThinking(arg); return true;
      case "model": {
        const [p, m] = arg.includes("/") ? arg.split("/", 2) : [s.state?.model?.provider ?? "", arg];
        await s.setModel(p, m);
        return true;
      }
      case "trust": await s.setTrust(arg === "no" ? "untrusted" : "trusted", false); return true;
    }
    return false;
  }

  async function submit(mode: "auto" | "followUp" = "auto") {
    const text = input.trim();
    if (!text) return;
    input = "";
    follow = true;
    if (text.startsWith("/") && (await slash(text))) return;
    if (mode === "followUp") await s.followUpPrompt(text);
    else await s.prompt(text);
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
      e.preventDefault();
      submit(e.altKey && s.running ? "followUp" : "auto");
    } else if (e.key === "Escape" && s.running) {
      e.preventDefault();
      s.abort();
    }
  }

  function autosize(el: HTMLTextAreaElement) {
    $effect(() => {
      void input;
      el.style.height = "auto";
      el.style.height = Math.min(el.scrollHeight, 260) + "px";
    });
  }

  // Open links from rendered markdown in the system browser.
  function onClick(e: MouseEvent) {
    const a = (e.target as HTMLElement).closest("a");
    if (a && a.href && /^https?:/.test(a.href)) {
      e.preventDefault();
      Browser.OpenURL(a.href);
    }
  }

  function blocks(m: Message): Block[] {
    return typeof m.content === "string" ? [{ type: "text", text: m.content }] : (m.content ?? []);
  }

  function fmtTokens(n: number) {
    if (!n) return "0";
    return n >= 1000 ? (n / 1000).toFixed(n >= 100000 ? 0 : 1) + "k" : String(n);
  }

  function ago(ms: number) {
    const d = (Date.now() - ms) / 1000;
    if (d < 60) return "just now";
    if (d < 3600) return Math.floor(d / 60) + "m ago";
    if (d < 86400) return Math.floor(d / 3600) + "h ago";
    return Math.floor(d / 86400) + "d ago";
  }

  const shortCwd = $derived(shortPath(s.cwd));
  const trust = $derived(s.state?.projectTrust);
  const sandbox = $derived(s.state?.sandbox);
  // malachi's code mode (off, on, explore); the choice is remembered like
  // the model and thinking level.
  const codeMode = $derived(s.state?.codeMode);
  const codeModeTitle = $derived.by(() => {
    const c = codeMode;
    if (!c) return "";
    const what: Record<string, string> = {
      off: "the script tool is not offered",
      on: "the model may chain tool calls in scripts",
      explore: "the model explores only through scripts",
    };
    let t = `Code mode ${c.mode}: ${what[c.mode] ?? c.mode}`;
    if (c.scripts) t += `\n${c.scripts} scripts (${c.failed} failed), ${c.calls} calls (${c.callErrors} failed) this session`;
    return t;
  });
  const ctxPct = $derived(s.stats?.contextUsage?.percent ?? 0);
  const modelKey = $derived(s.state ? `${s.state.model.provider}/${s.state.model.id}` : "");
  const modelOptions = $derived.by(() => {
    const opts = s.models.map((m) => `${m.provider}/${m.id}`);
    if (modelKey && !opts.includes(modelKey)) opts.unshift(modelKey);
    return opts;
  });
</script>

<svelte:window onclick={onClick} />

{#if !s.open && (s.starting || s.error)}
  <div class="welcome">
    <div class="card">
      <h1>malachi</h1>
      {#if s.starting}
        <p class="muted">Starting…</p>
      {:else}
        <p class="error">{s.error}</p>
        {#if s.stderr.length}<pre class="log">{s.stderr.join("\n")}</pre>{/if}
        <p class="muted small">malachi {s.version}</p>
        <div class="row">
          <button class="primary" onclick={() => s.init()}>Retry</button>
          <button onclick={() => openProject()}>Open project…</button>
        </div>
      {/if}
    </div>
  </div>
{:else}
  <div class="app">
    <aside class="side">
      <button class="place" class:active={s.isChat} disabled={s.running}
        onclick={() => !s.isChat && closeProject()} title="Talk with malachi without a project">
        <span class="place-icon">💬</span>
        <span class="place-name">Chat</span>
      </button>

      <div class="section">
        <span>Projects</span>
        <button class="sec-btn" onclick={() => openProject()} disabled={s.running} title="Open a project folder…">＋</button>
      </div>
      <div class="list">
        {#each projects as r (r.path)}
          {@const current = !s.isChat && r.path === s.cwd}
          <div class="item-row" class:active={current}>
            <button class="item" disabled={s.running || current} onclick={() => openProject(r.path)} title={r.path}>
              <span class="pv">{r.name}</span>
              <span class="when">{shortPath(r.path)}{r.lastUsed ? " · " + ago(r.lastUsed) : ""}</span>
            </button>
            {#if current}
              <button class="link x" onclick={closeProject} disabled={s.running} title="Close project">✕</button>
            {/if}
          </div>
        {:else}
          <button class="link small pad" onclick={() => openProject()}>Open a project…</button>
        {/each}
      </div>

      <div class="section">
        <span>{s.isChat ? "Chats" : `Sessions · ${projectName}`}</span>
        <button class="sec-btn" onclick={() => s.newSession()} disabled={s.running}
          title={s.isChat ? "New chat" : "New session"}>＋</button>
      </div>
      <div class="list">
        {#each s.sessions as ses (ses.path)}
          <button class="item" class:active={ses.path === s.state?.sessionFile}
            disabled={s.running} onclick={() => s.switchSession(ses.path)} title={ses.name}>
            <span class="pv">{ses.preview || "(empty)"}</span>
            <span class="when">{ago(ses.modified)}</span>
          </button>
        {:else}
          <div class="muted small pad">Nothing saved yet.</div>
        {/each}
      </div>

      {#if s.jobs.length}
        <div class="section">Jobs</div>
        {#each s.jobs as j (j.id)}
          <div class="job" title={j.log}>
            <span class="dot {j.status}"></span>
            <span class="pv">#{j.id} {j.description || j.command}</span>
          </div>
        {/each}
      {/if}

      {#if s.state?.mcp?.servers?.length}
        <div class="section">MCP</div>
        {#each s.state.mcp.servers as m}
          <div class="job" title={m.error || m.tools.join(", ")}>
            <span class="dot {m.status === 'ready' ? 'running' : m.status === 'failed' ? 'exited' : ''}"></span>
            <span class="pv">{m.name}</span>
            <span class="when">{m.tools.length} tools</span>
          </div>
        {/each}
      {/if}
      <div class="versions" title="malachi-desktop {s.appVersion}, with malachi {s.version} built in">
        malachi-desktop {s.appVersion} · malachi {s.version}
      </div>
    </aside>

    <main class="main">
      {#if s.running || s.compacting}<div class="scanner" aria-hidden="true"></div>{/if}
      {#if trust?.pending}
        <div class="banner">
          <span>This project has instruction files ({trust.files.join(", ")}) that are withheld until you decide.</span>
          <button onclick={() => s.setTrust("trusted", false)}>Trust once</button>
          <button onclick={() => s.setTrust("trusted", true)}>Always</button>
          <button onclick={() => s.setTrust("untrusted", true)}>Never</button>
        </div>
      {/if}
      {#if sandbox && (!sandbox.enabled || !sandbox.available)}
        <div class="banner warn">
          {#if !sandbox.enabled}Sandbox is off: commands can reach everything you can.
          {:else}Sandbox unavailable: {sandbox.reason}{/if}
        </div>
      {/if}

      <div class="transcript-wrap">
      <div class="transcript" bind:this={scroller} onscroll={onScroll}>
        <div class="inner">
          {#if !s.messages.length}
            <div class="empty muted">
              {#if s.isChat}What's on your mind? <span class="small">(Open a project from the sidebar to work on code.)</span>
              {:else}Ask malachi to do something in <code>{shortCwd}</code>.{/if}
            </div>
          {/if}
          {#each s.messages as m, i (i)}
            {#if m.role === "user"}
              <div class="msg user"><div class="bubble">{textOf(m.content)}</div></div>
            {:else if m.role === "assistant"}
              <div class="msg assistant">
                {#each blocks(m) as b}
                  {#if b.type === "text" && b.text}
                    <div class="md">{@html renderMarkdown(b.text)}</div>
                  {:else if b.type === "thinking" && (b.thinking || b.redacted)}
                    <details class="thinking">
                      <summary>💭 Thinking</summary>
                      <div class="tbody">{b.redacted ? "(redacted)" : b.thinking}</div>
                    </details>
                  {:else if b.type === "toolCall"}
                    <ToolCard name={b.name} args={b.arguments} result={results[b.id]} run={s.toolRuns[b.id]} />
                  {/if}
                {/each}
                {#if m.stopReason === "error" || m.errorMessage}
                  <div class="error">❌ {m.errorMessage || "The request failed."}</div>
                {:else if m.stopReason === "aborted"}
                  <div class="muted small">— stopped</div>
                {/if}
              </div>
            {:else if m.role === "compactionSummary"}
              <details class="note">
                <summary>📦 Earlier conversation compacted</summary>
                <div class="md">{@html renderMarkdown(m.summary ?? "")}</div>
              </details>
            {:else if m.role === "branchSummary"}
              <div class="note muted">↪ Branch summary</div>
            {:else if m.role === "bashExecution"}
              <ToolCard name="bash" args={{ command: m.command }} result={{ role: "toolResult", content: m.output ?? "" }} />
            {/if}
          {/each}
          {#if s.running && !s.compacting}
            <div class="working"><span class="pulse"></span> working…</div>
          {/if}
          {#if s.compacting}
            <div class="working"><span class="pulse"></span> {s.compactNote}</div>
          {/if}
        </div>
      </div>
      {#if !follow && s.messages.length}
        <button class="jump" class:new={unseen} onclick={jumpToBottom} title="Jump to the latest output">
          ↓ {unseen ? "New output" : "Jump to bottom"}
        </button>
      {/if}
      </div>

      <div class="composer">
        {#if s.error}<div class="error small">{s.error} <button class="link" onclick={() => (s.error = "")}>dismiss</button></div>{/if}
        {#if s.steering.length || s.followUp.length}
          <div class="queue">
            {#each s.steering as q}<span class="chip">steer: {q}</span>{/each}
            {#each s.followUp as q}<span class="chip">next: {q}</span>{/each}
          </div>
        {/if}
        <div class="box">
          <textarea bind:this={textarea} use:autosize bind:value={input} onkeydown={onKey} rows="1"
            placeholder={s.running ? "Steer, or Esc to stop" : "How can I help?"}></textarea>
          {#if s.running}
            <button class="stop" onclick={() => s.abort()} title="Stop (Esc)">■</button>
          {:else}
            <button class="send" onclick={() => submit()} disabled={!input.trim()} title="Send (Enter)">↑</button>
          {/if}
        </div>
      </div>

      <footer class="status">
        <select value={modelKey} onchange={(e) => {
          const v = (e.target as HTMLSelectElement).value; const k = v.indexOf("/");
          s.setModel(v.slice(0, k), v.slice(k + 1));
        }} disabled={s.running}>
          {#each modelOptions as o}<option value={o}>{o}</option>{/each}
        </select>
        {#if s.levels.length > 1}
          <select value={s.state?.thinkingLevel} onchange={(e) => s.setThinking((e.target as HTMLSelectElement).value)}>
            {#each s.levels as l}<option value={l}>think: {l}</option>{/each}
          </select>
        {/if}
        {#if codeMode?.mode && s.codeModes.length > 1}
          <select value={codeMode.mode} title={codeModeTitle}
            onchange={(e) => s.setCodeMode((e.target as HTMLSelectElement).value)}>
            {#each s.codeModes as m}<option value={m}>code: {m}</option>{/each}
          </select>
        {:else if codeMode?.mode}
          <span class="tag" class:dim={codeMode.mode === "off"} title={codeModeTitle}>code: {codeMode.mode}</span>
        {/if}
        {#if sandbox?.enabled}
          <button class="pill" class:on={sandbox.network} onclick={() => s.setNetwork(!sandbox.network)}
            title="Network access for sandboxed commands">net {sandbox.network ? "on" : "off"}</button>
        {/if}
        <button class="pill" onclick={() => s.compact()} disabled={s.running || s.compacting}>compact</button>
        <span class="spacer"></span>
        {#if s.stats}
          <span class="meter" title={`${s.stats.contextUsage.tokens} / ${s.stats.contextUsage.contextWindow} tokens`}>
            <span class="bar"><span class="fill" class:hot={ctxPct > 80} style:width={Math.min(ctxPct, 100) + "%"}></span></span>
            {ctxPct.toFixed(0)}%
          </span>
          <span class="muted">↑{fmtTokens(s.stats.tokens.input)} ↓{fmtTokens(s.stats.tokens.output)}</span>
          {#if s.stats.cost > 0}<span class="muted">${s.stats.cost.toFixed(3)}</span>{/if}
        {/if}
        <button class="pill" onclick={() => (showLog = !showLog)} title="malachi stderr">log</button>
      </footer>
      {#if showLog}
        <pre class="log floating">{s.stderr.join("\n") || "(nothing on stderr)"}</pre>
      {/if}
    </main>
  </div>
{/if}
