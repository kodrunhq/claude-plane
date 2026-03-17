import { useParams, useNavigate, NavLink } from 'react-router';
import { BookOpen, ChevronRight } from 'lucide-react';
import { MarkdownRenderer } from '../components/docs/MarkdownRenderer.tsx';

import gettingStarted from '../docs/getting-started.md?raw';
import telegramSetup from '../docs/telegram-setup.md?raw';
import githubSetup from '../docs/github-setup.md?raw';
import smtpSetup from '../docs/smtp-setup.md?raw';

interface Guide {
  readonly id: string;
  readonly title: string;
  readonly description: string;
  readonly content: string;
}

const guides: readonly Guide[] = [
  {
    id: 'getting-started',
    title: 'Getting Started',
    description: 'Overview of claude-plane and setup basics',
    content: gettingStarted,
  },
  {
    id: 'telegram-setup',
    title: 'Telegram Setup',
    description: 'Connect Telegram for notifications and commands',
    content: telegramSetup,
  },
  {
    id: 'github-setup',
    title: 'GitHub Setup',
    description: 'Automate sessions from GitHub events',
    content: githubSetup,
  },
  {
    id: 'smtp-setup',
    title: 'SMTP / Email Setup',
    description: 'Configure email notifications',
    content: smtpSetup,
  },
] as const;

export function DocsPage() {
  const { guideId } = useParams<{ guideId: string }>();
  const navigate = useNavigate();

  const activeId = guideId ?? guides[0].id;
  const activeGuide = guides.find((g) => g.id === activeId) ?? guides[0];

  return (
    <div className="flex h-full overflow-hidden">
      {/* Sidebar TOC */}
      <nav className="w-64 shrink-0 border-r border-border-primary bg-bg-secondary overflow-y-auto hidden md:block">
        <div className="p-4">
          <div className="flex items-center gap-2 mb-4">
            <BookOpen size={18} className="text-accent-primary" />
            <h2 className="text-sm font-semibold text-text-primary">Documentation</h2>
          </div>
          <ul className="space-y-1">
            {guides.map((guide) => (
              <li key={guide.id}>
                <NavLink
                  to={`/docs/${guide.id}`}
                  className={({ isActive }) =>
                    `w-full text-left px-3 py-2 rounded-lg text-sm transition-all duration-150 flex items-center gap-2 block ${
                      isActive
                        ? 'bg-accent-primary/10 text-accent-primary font-medium'
                        : 'text-text-secondary hover:text-text-primary hover:bg-bg-tertiary/60'
                    }`
                  }
                >
                  {({ isActive }) => (
                    <>
                      {isActive && <ChevronRight size={14} className="shrink-0" />}
                      <div className="min-w-0">
                        <div className="truncate">{guide.title}</div>
                        {isActive && (
                          <div className="text-xs text-text-secondary/70 mt-0.5 truncate">{guide.description}</div>
                        )}
                      </div>
                    </>
                  )}
                </NavLink>
              </li>
            ))}
          </ul>
        </div>
      </nav>

      {/* Mobile guide selector */}
      <div className="md:hidden fixed top-16 left-0 right-0 z-10 bg-bg-secondary border-b border-border-primary px-4 py-2">
        <select
          value={activeId}
          onChange={(e) => navigate(`/docs/${e.target.value}`)}
          className="w-full px-3 py-2 rounded-lg text-sm bg-bg-tertiary text-text-primary border border-border-primary"
        >
          {guides.map((guide) => (
            <option key={guide.id} value={guide.id}>{guide.title}</option>
          ))}
        </select>
      </div>

      {/* Content area */}
      <main className="flex-1 overflow-y-auto p-6 md:p-8 lg:p-10">
        <div className="max-w-3xl">
          <MarkdownRenderer content={activeGuide.content} />
        </div>
      </main>
    </div>
  );
}
