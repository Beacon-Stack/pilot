import type { ReactNode } from "react";

// Inline "external link" icon so this file has no dependency on lucide-react.
// The shared package keeps its dep surface to React only — each consuming
// service already has its own copy of lucide, but we don't want web-shared
// to reach into sibling node_modules for it.
function ExternalLinkIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="12"
      height="12"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      style={{ verticalAlign: "-1px" }}
    >
      <path d="M15 3h6v6" />
      <path d="M10 14 21 3" />
      <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h6" />
    </svg>
  );
}

interface PageHeaderProps {
  title: string;
  description: string;
  docsUrl?: string;
  action?: ReactNode;
}

export default function PageHeader({ title, description, docsUrl, action }: PageHeaderProps) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "flex-start",
        justifyContent: "space-between",
        marginBottom: 24,
        // flexWrap: "wrap" lets the action button drop below the
        // title block on narrow viewports instead of overlapping or
        // pushing the title's line-wrapped text against the action.
        // gap of 12px + minWidth: 0 on the title block ensures clean
        // multi-line wrapping in either layout.
        flexWrap: "wrap",
        gap: 12,
      }}
    >
      <div style={{ flex: 1, minWidth: 0 }}>
        <h1 style={{ margin: 0, fontSize: 20, fontWeight: 600, color: "var(--color-text-primary)", letterSpacing: "-0.01em" }}>
          {title}
        </h1>
        <p style={{ margin: "4px 0 0", fontSize: 13, color: "var(--color-text-secondary)" }}>
          {description}
          {docsUrl && (
            <>
              {" "}
              <a
                href={docsUrl}
                target="_blank"
                rel="noopener noreferrer"
                style={{
                  color: "var(--color-accent)",
                  textDecoration: "none",
                  fontSize: 13,
                  whiteSpace: "nowrap",
                }}
              >
                Learn more <ExternalLinkIcon />
              </a>
            </>
          )}
        </p>
      </div>
      {action}
    </div>
  );
}
