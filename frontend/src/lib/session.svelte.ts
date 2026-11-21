// Client-side model of one malachi session, fed by the RPC event stream.
import { Events } from "@wailsio/runtime";
import { MalachiService } from "../../bindings/github.com/ddombrow/malachi-desktop";
import type { SessionInfo, RecentProject } from "../../bindings/github.com/ddombrow/malachi-desktop/models";

export type Block =
  | { type: "text"; text: string }
  | { type: "thinking"; thinking: string; redacted?: boolean }
  | { type: "image"; data: string; mimeType: string }
  | { type: "toolCall"; id: string; name: string; arguments: Record<string, any> };

export interface Message {
  role: string;
  content: string | Block[];
  // assistant
  model?: string;
  stopReason?: string;
  errorMessage?: string;
  usage?: { input: number; output: number; totalTokens: number; cost?: { total: number } };
  // toolResult
  toolCallId?: string;
  toolName?: string;
  isError?: boolean;
  details?: any;
  // others (compactionSummary, bashExecution, …)
  summary?: string;
  [k: string]: any;
}

export interface ToolRun {
  status: "running" | "done" | "error";
  partial?: string;
  details?: any;
}

export interface Job {
  id: number;
  command: string;
  description?: string;
  status: string;
  exitCode?: number;
  startedAt: number;
  log: string;
}

export interface Model {
  id: string;
  provider: string;
  reasoning: boolean;
  contextWindow: number;
}

function textOf(content: string | Block[] | undefined): string {
  if (!content) return "";
  if (typeof content === "string") return content;
  return content
    .filter((b) => b.type === "text")
    .map((b: any) => b.text)
    .join("");
}
export { textOf };

const send = (type: string, fields: Record<string, any> = {}) =>
  MalachiService.Send({ type, ...fields }) as Promise<any>;

class Session {
  // connection
  open = $state(false);
  cwd = $state("");
  chatDir = $state("");
  starting = $state(true);
  version = $state(""); // malachi's, built into the app
  appVersion = $state(""); // the desktop app's
  error = $state("");
  stderr = $state<string[]>([]);

  // transcript
  messages = $state<Message[]>([]);
  toolRuns = $state<Record<string, ToolRun>>({});

  // agent state
  running = $state(false);
  compacting = $state(false);
  compactNote = $state("");
  steering = $state<string[]>([]);
  followUp = $state<string[]>([]);

  // session state (get_state)
  state = $state<any>(null);
  stats = $state<any>(null);
  models = $state<Model[]>([]);
  levels = $state<string[]>([]);
  codeModes = $state<string[]>([]);
  jobs = $state<Job[]>([]);
  sessions = $state<SessionInfo[]>([]);
  recent = $state<RecentProject[]>([]);

  #unsubs: (() => void)[] = [];
  #statsTimer: ReturnType<typeof setTimeout> | undefined;

  constructor() {
    this.#unsubs.push(
      Events.On("malachi:event", (ev: any) => {
        try {
          this.#handle(JSON.parse(ev.data));
        } catch (e) {
          console.error("bad event", e, ev.data);
        }
      }),
      Events.On("malachi:stderr", (ev: any) => {
        this.stderr = [...this.stderr.slice(-199), ev.data];
      }),
      Events.On("malachi:exit", (ev: any) => {
        this.open = false;
        this.running = false;
        this.error = `malachi stopped: ${ev.data}`;
      }),
    );
  }

  // A chat without a project runs in the app's own empty directory.
  get isChat() {
    return !!this.chatDir && this.cwd === this.chatDir;
  }

  async init() {
    try {
      const info = await MalachiService.Info();
      this.version = info.version;
      this.appVersion = info.appVersion;
      this.chatDir = info.chatDir;
      if (info.initErr) this.error = info.initErr;
      else if (info.chatDirErr) this.error = info.chatDirErr;
      if (info.running) {
        this.cwd = info.cwd;
        this.open = true;
        await this.refreshAll();
      } else if (!this.error) {
        await this.openChat();
      }
    } finally {
      this.starting = false;
    }
  }

  // Start a fresh project-less chat; past ones are in the sidebar.
  openChat() {
    return this.openProject(this.chatDir);
  }

  async openProject(cwd: string, opts: { continue?: boolean; resume?: string } = {}) {
    this.error = "";
    this.stderr = [];
    this.messages = [];
    this.toolRuns = {};
    this.running = false;
    this.compacting = false;
    try {
      await MalachiService.Open({
        cwd,
        continue: !!opts.continue,
        resume: opts.resume ?? "",
        model: "",
        thinking: "",
      });
      this.cwd = cwd;
      this.open = true;
      await this.refreshAll();
    } catch (e: any) {
      this.error = String(e?.message ?? e);
      this.open = false;
    }
  }

  async refreshAll() {
    await Promise.all([this.refreshState(), this.loadMessages()]);
    this.refreshStats();
    send("get_available_thinking_levels").then((d) => (this.levels = d?.levels ?? []));
    send("get_available_code_modes")
      .then((d) => (this.codeModes = d?.modes ?? []))
      .catch(() => (this.codeModes = []));
    send("get_available_models")
      .then((d) => (this.models = d?.models ?? []))
      .catch(() => {});
    this.refreshSessions();
  }

  async refreshState() {
    const st = await send("get_state");
    this.state = st;
    this.running = st.isStreaming;
    this.compacting = st.isCompacting;
    this.jobs = st.jobs ?? [];
  }

  refreshStats() {
    clearTimeout(this.#statsTimer);
    this.#statsTimer = setTimeout(async () => {
      try {
        this.stats = await send("get_session_stats");
      } catch {}
    }, 150);
  }

  async refreshRecent() {
    try {
      this.recent = (await MalachiService.RecentProjects()) ?? [];
    } catch {
      this.recent = [];
    }
  }

  async refreshSessions() {
    this.refreshRecent();
    try {
      this.sessions = (await MalachiService.ListSessions(this.cwd)) ?? [];
    } catch {
      this.sessions = [];
    }
  }

  async loadMessages() {
    const d = await send("get_messages");
    this.messages = d?.messages ?? [];
    this.toolRuns = {};
  }

  // ---- commands ----

  async prompt(text: string) {
    text = text.trim();
    if (!text) return;
    this.error = "";
    try {
      if (this.running || this.compacting) {
        await send("prompt", { message: text, streamingBehavior: "steer" });
      } else {
        await send("prompt", { message: text });
      }
    } catch (e: any) {
      this.error = String(e?.message ?? e);
    }
  }

  async followUpPrompt(text: string) {
    text = text.trim();
    if (!text) return;
    try {
      await send("prompt", { message: text, streamingBehavior: "followUp" });
    } catch (e: any) {
      this.error = String(e?.message ?? e);
    }
  }

  abort() {
    send("abort").catch(() => {});
  }

  async run(type: string, fields: Record<string, any> = {}) {
    this.error = "";
    try {
      return await send(type, fields);
    } catch (e: any) {
      this.error = String(e?.message ?? e);
    }
  }

  // Model and thinking choices are remembered by the app (not malachi) and
  // applied to every session it starts.
  async setModel(provider: string, modelId: string) {
    this.error = "";
    try {
      await send("set_model", { provider, modelId });
      await MalachiService.UpdateSettings({ model: provider ? `${provider}/${modelId}` : modelId, thinking: "", codeMode: "" });
    } catch (e: any) {
      this.error = String(e?.message ?? e);
    }
    await this.refreshState();
    this.refreshStats();
  }

  async setThinking(level: string) {
    this.error = "";
    try {
      await send("set_thinking_level", { level });
      await MalachiService.UpdateSettings({ model: "", thinking: level, codeMode: "" });
    } catch (e: any) {
      this.error = String(e?.message ?? e);
    }
    await this.refreshState();
  }

  async setCodeMode(mode: string) {
    this.error = "";
    try {
      const cm = await send("set_code_mode", { mode });
      if (this.state && cm) this.state.codeMode = cm;
      await MalachiService.UpdateSettings({ model: "", thinking: "", codeMode: mode });
    } catch (e: any) {
      this.error = String(e?.message ?? e);
    }
  }

  async setNetwork(enabled: boolean) {
    await this.run("set_network", { enabled });
    await this.refreshState();
  }

  async setTrust(decision: "trusted" | "untrusted", remember: boolean) {
    await this.run("set_trust", { decision, remember });
    await this.refreshState();
  }

  async allowRoot(kind: "read" | "write", path: string) {
    await this.run(kind === "read" ? "add_readable_root" : "add_writable_root", { path });
    await this.refreshState();
  }

  async compact(note = "") {
    await this.run("compact", note ? { customInstructions: note } : {});
    await this.loadMessages();
    this.refreshStats();
  }

  async newSession() {
    await this.run("new_session");
    await this.refreshAll();
  }

  async switchSession(path: string) {
    await this.run("switch_session", { sessionPath: path });
    // A resumed session brings back the model it was using; the user's saved
    // choice wins, as it does for sessions the app starts.
    await this.applySaved();
    await this.refreshAll();
  }

  async applySaved() {
    try {
      const saved = await MalachiService.GetSettings();
      const st = await send("get_state");
      const cur = `${st.model.provider}/${st.model.id}`;
      if (saved.model && saved.model !== cur) {
        const k = saved.model.indexOf("/");
        await send("set_model", k > 0
          ? { provider: saved.model.slice(0, k), modelId: saved.model.slice(k + 1) }
          : { modelId: saved.model });
      }
      if (saved.thinking && saved.thinking !== st.thinkingLevel) {
        await send("set_thinking_level", { level: saved.thinking });
      }

    } catch {
      // A saved model the provider no longer offers leaves the session as is.
    }
  }

  async close() {
    await MalachiService.Close();
    this.open = false;
  }

  // ---- events ----

  #handle(e: any) {
    switch (e.type) {
      case "agent_start":
        this.running = true;
        break;
      case "agent_end":
        break;
      case "agent_settled":
        this.running = false;
        this.refreshStats();
        this.refreshState().then(() => this.refreshSessions());
        break;
      case "message_start":
        this.messages.push(e.message);
        break;
      case "message_update":
      case "message_end": {
        const i = this.#findStreaming(e.message);
        if (i >= 0) this.messages[i] = e.message;
        else this.messages.push(e.message);
        if (e.type === "message_end" && e.message.role === "assistant") this.refreshStats();
        break;
      }
      case "tool_execution_start":
        this.toolRuns[e.toolCallId] = { status: "running" };
        break;
      case "tool_execution_update":
        this.toolRuns[e.toolCallId] = {
          status: "running",
          partial: textOf(e.partialResult?.content),
          details: e.partialResult?.details,
        };
        break;
      case "tool_execution_end":
        this.toolRuns[e.toolCallId] = {
          status: e.isError ? "error" : "done",
          details: e.result?.details,
        };
        break;
      case "queue_update":
        this.steering = e.steering ?? [];
        this.followUp = e.followUp ?? [];
        break;
      case "compaction_start":
        this.compacting = true;
        this.compactNote = "Compacting…";
        break;
      case "compaction_progress":
        this.compactNote = e.message ?? e.phase ?? "Compacting…";
        break;
      case "compaction_end":
        this.compacting = false;
        this.compactNote = "";
        this.loadMessages();
        this.refreshStats();
        break;
      case "thinking_level_changed":
        if (this.state) this.state.thinkingLevel = e.level;
        break;
      case "sandbox_changed":
        if (this.state) this.state.sandbox = e.sandbox;
        break;
      case "mcp_changed":
        if (this.state) this.state.mcp = e.mcp;
        break;
      case "jobs_changed":
        this.jobs = e.jobs ?? [];
        break;
      case "code_mode_changed":
        if (this.state) this.state.codeMode = e.codeMode;
        break;
    }
  }

  // The message an update belongs to: the last one of the same role
  // (updates always concern the message most recently started).
  #findStreaming(m: Message): number {
    for (let i = this.messages.length - 1; i >= 0; i--) {
      const cur = this.messages[i];
      if (cur.role !== m.role) continue;
      if (m.role === "toolResult" && cur.toolCallId !== m.toolCallId) continue;
      return i;
    }
    return -1;
  }
}

export const session = new Session();
