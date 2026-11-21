import { marked } from "marked";
import DOMPurify from "dompurify";

marked.setOptions({ gfm: true, breaks: false });

// Model output is untrusted: the webview can call Go, so nothing it renders
// may run script. Links open through the click handler in App.
export function renderMarkdown(src: string): string {
  const html = marked.parse(src, { async: false }) as string;
  return DOMPurify.sanitize(html, { FORBID_TAGS: ["style", "form", "input", "iframe"], FORBID_ATTR: ["style"] });
}
