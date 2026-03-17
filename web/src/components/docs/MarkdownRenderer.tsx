import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Link } from 'react-router';

interface MarkdownRendererProps {
  content: string;
}

export function MarkdownRenderer({ content }: MarkdownRendererProps) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        h1: ({ children }) => (
          <h1 className="text-2xl font-bold text-text-primary mt-8 mb-4 first:mt-0">{children}</h1>
        ),
        h2: ({ children }) => (
          <h2 className="text-xl font-semibold text-text-primary mt-6 mb-3 pb-2 border-b border-border-primary">{children}</h2>
        ),
        h3: ({ children }) => (
          <h3 className="text-lg font-medium text-text-primary mt-4 mb-2">{children}</h3>
        ),
        p: ({ children }) => (
          <p className="text-sm text-text-secondary leading-relaxed mb-3">{children}</p>
        ),
        a: ({ href, children }) => {
          if (href?.startsWith('/docs/') || href?.startsWith('/')) {
            return (
              <Link to={href} className="text-accent-primary hover:underline">
                {children}
              </Link>
            );
          }
          return (
            <a href={href} className="text-accent-primary hover:underline" target="_blank" rel="noopener noreferrer">
              {children}
            </a>
          );
        },
        code: ({ className, children }) => {
          const isBlock = className?.startsWith('language-');
          if (isBlock) {
            return (
              <pre className="bg-bg-tertiary rounded-lg p-4 overflow-x-auto my-3">
                <code className="text-xs font-mono text-text-primary">{children}</code>
              </pre>
            );
          }
          return (
            <code className="bg-bg-tertiary px-1.5 py-0.5 rounded text-xs font-mono text-accent-primary">
              {children}
            </code>
          );
        },
        pre: ({ children }) => <>{children}</>,
        ul: ({ children }) => (
          <ul className="list-disc pl-6 mb-3 space-y-1 text-sm text-text-secondary">{children}</ul>
        ),
        ol: ({ children }) => (
          <ol className="list-decimal pl-6 mb-3 space-y-1 text-sm text-text-secondary">{children}</ol>
        ),
        table: ({ children }) => (
          <div className="overflow-x-auto my-3">
            <table className="w-full text-sm border-collapse border border-border-primary">{children}</table>
          </div>
        ),
        th: ({ children }) => (
          <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-text-secondary bg-bg-tertiary border border-border-primary">
            {children}
          </th>
        ),
        td: ({ children }) => (
          <td className="px-3 py-2 text-sm text-text-secondary border border-border-primary">{children}</td>
        ),
        blockquote: ({ children }) => (
          <blockquote className="border-l-4 border-accent-primary pl-4 my-3 text-sm text-text-secondary italic">
            {children}
          </blockquote>
        ),
      }}
    >
      {content}
    </ReactMarkdown>
  );
}
