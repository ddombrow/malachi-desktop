// Parse a git-style unified patch (malachi's WorkingTreeDiff, or edit's
// own) into files, hunks and numbered lines, and find the words a changed
// line changed.

export type LineKind = "ctx" | "add" | "del" | "note";

// A run of a line's text; changed runs are the words a paired line differs in.
export interface Seg {
  text: string;
  changed: boolean;
}

export interface DiffLine {
  kind: LineKind;
  text: string; // without the leading +, - or space
  oldNo?: number;
  newNo?: number;
  segs?: Seg[]; // set on a changed line paired with its counterpart
}

export interface Hunk {
  header: string; // the @@ line
  lines: DiffLine[];
}

export type FileStatus = "A" | "D" | "R" | "M";

export interface DiffFile {
  path: string; // the new path, or the old one for a deletion
  oldPath?: string; // set on a rename
  status: FileStatus;
  binary: boolean;
  adds: number;
  dels: number;
  hunks: Hunk[];
  raw: string; // this file's part of the patch
}

// unquote undoes git's C-style quoting of a path ("a/caf\303\251").
function unquote(p: string): string {
  if (!p.startsWith('"') || !p.endsWith('"')) return p;
  const bytes: number[] = [];
  const s = p.slice(1, -1);
  const esc: Record<string, number> = { n: 10, t: 9, r: 13, b: 8, f: 12, v: 11, a: 7, '"': 34, "\\": 92 };
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (c !== "\\") {
      bytes.push(...new TextEncoder().encode(c));
      continue;
    }
    const n = s[i + 1];
    if (/[0-7]/.test(n)) {
      bytes.push(parseInt(s.slice(i + 1, i + 4), 8));
      i += 3;
    } else {
      bytes.push(esc[n] ?? n.charCodeAt(0));
      i += 1;
    }
  }
  return new TextDecoder().decode(new Uint8Array(bytes));
}

// stripPrefix drops git's a/ or b/ from a path, or returns "" for /dev/null.
function stripPrefix(p: string): string {
  p = unquote(p.replace(/\t.*$/, ""));
  if (p === "/dev/null") return "";
  return p.replace(/^[ab]\//, "");
}

// pathsFromDiffLine reads "diff --git a/x b/y", for files whose ---/+++
// lines are missing (binary, mode-only, empty).
function pathsFromDiffLine(line: string): [string, string] {
  const rest = line.slice("diff --git ".length);
  if (rest.startsWith('"')) {
    const m = rest.match(/^("(?:[^"\\]|\\.)*")\s+(.*)$/);
    if (m) return [stripPrefix(m[1]), stripPrefix(m[2])];
  }
  // Unquoted: the paths are the same unless renamed, so split in the middle.
  const half = rest.length / 2;
  const i = rest.indexOf(" b/", Math.floor(half) - 2);
  if (i >= 0) return [stripPrefix(rest.slice(0, i)), stripPrefix(rest.slice(i + 1))];
  return [stripPrefix(rest), stripPrefix(rest)];
}

export function parsePatch(patch: string): DiffFile[] {
  const files: DiffFile[] = [];
  const lines = patch.split("\n");
  if (lines[lines.length - 1] === "") lines.pop();

  let f: DiffFile | null = null;
  let start = 0;
  let h: Hunk | null = null;
  let oldNo = 0, newNo = 0;
  let oldLeft = 0, newLeft = 0; // lines left in the hunk on each side

  const finish = (end: number) => {
    if (!f) return;
    f.raw = lines.slice(start, end).join("\n") + "\n";
    for (const hk of f.hunks) pairWords(hk);
    files.push(f);
  };

  for (let i = 0; i < lines.length; i++) {
    const l = lines[i];
    const inHunk = h && (oldLeft > 0 || newLeft > 0);
    if (inHunk && (l[0] === " " || l[0] === "+" || l[0] === "-" || l === "")) {
      const text = l.slice(1);
      if (l[0] === "+") {
        h!.lines.push({ kind: "add", text, newNo: newNo++ });
        f!.adds++;
        newLeft--;
      } else if (l[0] === "-") {
        h!.lines.push({ kind: "del", text, oldNo: oldNo++ });
        f!.dels++;
        oldLeft--;
      } else {
        h!.lines.push({ kind: "ctx", text, oldNo: oldNo++, newNo: newNo++ });
        oldLeft--;
        newLeft--;
      }
      continue;
    }
    if (l.startsWith("\\")) {
      h?.lines.push({ kind: "note", text: l.slice(2) });
      continue;
    }
    if (l.startsWith("diff --git ")) {
      finish(i);
      const [a, b] = pathsFromDiffLine(l);
      f = { path: b || a, status: "M", binary: false, adds: 0, dels: 0, hunks: [], raw: "" };
      if (a && b && a !== b) f.oldPath = a;
      start = i;
      h = null;
      continue;
    }
    if (!f) continue;
    const hm = l.match(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/);
    if (hm) {
      oldNo = +hm[1];
      newNo = +hm[3];
      oldLeft = hm[2] === undefined ? 1 : +hm[2];
      newLeft = hm[4] === undefined ? 1 : +hm[4];
      h = { header: l, lines: [] };
      f.hunks.push(h);
      continue;
    }
    if (l.startsWith("new file mode")) f.status = "A";
    else if (l.startsWith("deleted file mode")) f.status = "D";
    else if (l.startsWith("rename from ")) { f.oldPath = unquote(l.slice(12)); f.status = "R"; }
    else if (l.startsWith("rename to ")) { f.path = unquote(l.slice(10)); f.status = "R"; }
    else if (l.startsWith("Binary files ") || l === "GIT binary patch") f.binary = true;
    else if (l.startsWith("--- ")) { const p = stripPrefix(l.slice(4)); if (p && f.status === "D") f.path = p; }
    else if (l.startsWith("+++ ")) { const p = stripPrefix(l.slice(4)); if (p) f.path = p; }
  }
  finish(lines.length);
  for (const file of files) {
    if (file.oldPath === file.path) file.oldPath = undefined;
    if (file.oldPath && file.status === "M") file.status = "R";
  }
  return files;
}

// ---- changed words ----

const maxPairChars = 400;

function tokenize(s: string): string[] {
  return s.match(/\w+|\s+|[^\w\s]/g) ?? [];
}

// diffTokens marks the tokens of a and b outside their longest common
// subsequence, merged into runs.
function diffTokens(a: string[], b: string[]): [Seg[], Seg[], number] {
  const n = a.length, m = b.length;
  const dp: Uint16Array[] = Array.from({ length: n + 1 }, () => new Uint16Array(m + 1));
  for (let i = n - 1; i >= 0; i--)
    for (let j = m - 1; j >= 0; j--)
      dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
  const sa: Seg[] = [], sb: Seg[] = [];
  const push = (segs: Seg[], text: string, changed: boolean) => {
    const last = segs[segs.length - 1];
    if (last && last.changed === changed) last.text += text;
    else segs.push({ text, changed });
  };
  let i = 0, j = 0, same = 0;
  while (i < n || j < m) {
    if (i < n && j < m && a[i] === b[j]) {
      same += a[i].length;
      push(sa, a[i++], false);
      push(sb, b[j++], false);
    } else if (j < m && (i === n || dp[i][j + 1] >= dp[i + 1][j])) {
      push(sb, b[j++], true);
    } else {
      push(sa, a[i++], true);
    }
  }
  return [sa, sb, same];
}

// pairWords pairs each run of removed lines with the added lines after it,
// line by line, and marks the words that changed where the two are alike
// enough for that to help.
function pairWords(h: Hunk) {
  const ls = h.lines;
  for (let i = 0; i < ls.length; ) {
    if (ls[i].kind !== "del") { i++; continue; }
    let d = i;
    while (d < ls.length && ls[d].kind === "del") d++;
    let a = d;
    while (a < ls.length && ls[a].kind === "add") a++;
    const pairs = Math.min(d - i, a - d);
    for (let k = 0; k < pairs; k++) {
      const del = ls[i + k], add = ls[d + k];
      if (del.text.length > maxPairChars || add.text.length > maxPairChars) continue;
      const [sd, sa, same] = diffTokens(tokenize(del.text), tokenize(add.text));
      const longer = Math.max(del.text.trim().length, add.text.trim().length);
      if (longer === 0 || same / Math.max(del.text.length, add.text.length) < 0.4) continue;
      if (!sd.some((s) => s.changed) && !sa.some((s) => s.changed)) continue;
      del.segs = sd;
      add.segs = sa;
    }
    i = Math.max(a, i + 1);
  }
}
