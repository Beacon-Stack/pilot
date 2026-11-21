import type { ReactNode } from "react";

/**
 * Renders the markdown Beacon release notes use: headings, bullet lists,
 * paragraphs, **bold**, `code`, fenced code blocks, [links](https://…),
 * bare https:// links, and <details>/<summary> folds.
 *
 * It builds React elements and never sets innerHTML, so nothing in the
 * notes can inject markup or script; only https links are made clickable.
 * Anything it doesn't understand comes out as plain text.
 */
export default function Markdown({ text }: { text: string }) {
  return <div style={{ fontSize: 13, lineHeight: 1.55, color: "var(--color-text-secondary)" }}>{renderBlocks(parse(text))}</div>;
}

type Block =
  | { kind: "heading"; level: number; text: string }
  | { kind: "list"; items: string[] }
  | { kind: "para"; text: string }
  | { kind: "code"; text: string }
  | { kind: "details"; summary: string; children: Block[] };

function parse(text: string): Block[] {
  const lines = text.replace(/\r\n/g, "\n").split("\n");
  const root: Block[] = [];
  // Open <details> folds, innermost last.
  const stack: { summary: string; children: Block[] }[] = [];
  const out = () => (stack.length ? stack[stack.length - 1].children : root);
  let para: string[] = [];
  let list: string[] | null = null;

  const flush = () => {
    if (para.length) out().push({ kind: "para", text: para.join(" ") });
    if (list) out().push({ kind: "list", items: list });
    para = [];
    list = null;
  };

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const trimmed = line.trim();
    if (trimmed.startsWith("```")) {
      flush();
      const code: string[] = [];
      for (i++; i < lines.length && !lines[i].trim().startsWith("```"); i++) code.push(lines[i]);
      out().push({ kind: "code", text: code.join("\n") });
      continue;
    }
    if (/^<details>$/i.test(trimmed)) {
      flush();
      stack.push({ summary: "", children: [] });
      continue;
    }
    const summary = trimmed.match(/^<summary>(.*)<\/summary>$/i);
    if (summary && stack.length) {
      stack[stack.length - 1].summary = summary[1];
      continue;
    }
    if (/^<\/details>$/i.test(trimmed)) {
      flush();
      const fold = stack.pop();
      if (fold) out().push({ kind: "details", summary: fold.summary || "Details", children: fold.children });
      continue;
    }
    if (trimmed === "") {
      flush();
      continue;
    }
    const heading = trimmed.match(/^(#{1,6})\s+(.*)$/);
    if (heading) {
      flush();
      out().push({ kind: "heading", level: heading[1].length, text: heading[2] });
      continue;
    }
    const item = trimmed.match(/^[-*]\s+(.*)$/);
    if (item) {
      if (para.length) flush();
      list = list ?? [];
      list.push(item[1]);
      continue;
    }
    if (list) flush();
    para.push(trimmed);
  }
  flush();
  // Unclosed folds still show their contents.
  while (stack.length) {
    const fold = stack.pop()!;
    out().push({ kind: "details", summary: fold.summary || "Details", children: fold.children });
  }
  return root;
}

function renderBlocks(blocks: Block[]): ReactNode[] {
  return blocks.map((b, i) => {
    switch (b.kind) {
      case "heading":
        return (
          <p
            key={i}
            style={{
              margin: "14px 0 6px",
              fontSize: b.level <= 2 ? 14 : 13,
              fontWeight: 600,
              color: "var(--color-text-primary)",
            }}
          >
            {inline(b.text)}
          </p>
        );
      case "list":
        return (
          <ul key={i} style={{ margin: "4px 0 8px", paddingLeft: 18 }}>
            {b.items.map((it, j) => (
              <li key={j} style={{ margin: "2px 0" }}>
                {inline(it)}
              </li>
            ))}
          </ul>
        );
      case "code":
        return (
          <pre
            key={i}
            style={{
              margin: "6px 0 10px",
              padding: 10,
              borderRadius: 6,
              background: "var(--color-bg-elevated)",
              border: "1px solid var(--color-border-subtle)",
              fontFamily: "var(--font-family-mono)",
              fontSize: 12,
              whiteSpace: "pre-wrap",
              color: "var(--color-text-primary)",
            }}
          >
            {b.text}
          </pre>
        );
      case "details":
        return (
          <details key={i} style={{ margin: "6px 0 10px" }}>
            <summary style={{ cursor: "pointer", color: "var(--color-text-muted)" }}>{inline(b.summary)}</summary>
            <div style={{ paddingLeft: 12 }}>{renderBlocks(b.children)}</div>
          </details>
        );
      default:
        return (
          <p key={i} style={{ margin: "6px 0" }}>
            {inline(b.text)}
          </p>
        );
    }
  });
}

const tokenRe = /(\*\*[^*]+\*\*|`[^`]+`|\[[^\]]+\]\([^)\s]+\)|https:\/\/[^\s)<]+)/g;

function inline(text: string): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  for (const m of text.matchAll(tokenRe)) {
    const tok = m[0];
    const at = m.index ?? 0;
    if (at > last) out.push(text.slice(last, at));
    const key = `${at}`;
    if (tok.startsWith("**")) {
      out.push(<strong key={key} style={{ color: "var(--color-text-primary)" }}>{tok.slice(2, -2)}</strong>);
    } else if (tok.startsWith("`")) {
      out.push(
        <code key={key} style={{ fontFamily: "var(--font-family-mono)", fontSize: 12 }}>
          {tok.slice(1, -1)}
        </code>,
      );
    } else if (tok.startsWith("[")) {
      const lm = tok.match(/^\[([^\]]+)\]\(([^)\s]+)\)$/);
      out.push(lm ? link(key, lm[1], lm[2]) : tok);
    } else {
      out.push(link(key, tok, tok));
    }
    last = at + tok.length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

function link(key: string, label: string, href: string): ReactNode {
  if (!/^https:\/\//i.test(href)) return label; // only https links are made clickable
  return (
    <a key={key} href={href} target="_blank" rel="noreferrer" style={{ color: "var(--color-accent)" }}>
      {label}
    </a>
  );
}
