import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSanitize from "rehype-sanitize";

// Markdown renders a safe subset: react-markdown does not emit raw HTML by
// default, rehype-sanitize strips anything dangerous, and links open in a new
// tab with noopener. External images are not loaded (no img handling here).
export function Markdown({ children }: { children: string }) {
  return (
    <div className="markdown-body text-sm">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeSanitize]}
        components={{
          a: ({ href, children }) => {
            const safe = href && /^https?:\/\//i.test(href);
            if (!safe) return <span>{children}</span>;
            return (
              <a href={href} target="_blank" rel="noopener noreferrer">
                {children}
              </a>
            );
          },
          img: () => null,
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  );
}
