import { describe, it, expect } from 'vitest';
import { renderWithProviders, screen } from '../../../test/render.tsx';
import { TemplateCard } from '../../../components/templates/TemplateCard.tsx';
import { buildTemplate } from '../../../test/factories.ts';
import type { SessionTemplate } from '../../../types/template.ts';

describe('TemplateCard', () => {
  function renderCard(overrides?: Partial<SessionTemplate>) {
    const template = buildTemplate({
      template_id: 'tmpl-card-1',
      name: 'My Claude Template',
      ...overrides,
    });
    return renderWithProviders(<TemplateCard template={template} />);
  }

  it('displays template name', () => {
    renderCard({ name: 'Deploy Bot' });
    expect(screen.getByText('Deploy Bot')).toBeInTheDocument();
  });

  it('displays description when present', () => {
    renderCard({ description: 'Deploys the bot to production' });
    expect(screen.getByText('Deploys the bot to production')).toBeInTheDocument();
  });

  it('does not display description paragraph when not present', () => {
    renderCard({ description: undefined });
    // Only the name should be in the heading area
    expect(screen.queryByText('Deploys the bot to production')).not.toBeInTheDocument();
  });

  it('displays command info when command is set', () => {
    renderCard({ command: 'claude' });
    expect(screen.getByText('cmd')).toBeInTheDocument();
    expect(screen.getByText('claude')).toBeInTheDocument();
  });

  it('displays command with args', () => {
    renderCard({ command: 'npm', args: ['run', 'build'] });
    expect(screen.getByText('npm run build')).toBeInTheDocument();
  });

  it('does not show command section when command is not set', () => {
    renderCard({ command: undefined });
    expect(screen.queryByText('cmd')).not.toBeInTheDocument();
  });

  it('displays working directory when set', () => {
    renderCard({ working_dir: '/home/user/project' });
    expect(screen.getByText('dir')).toBeInTheDocument();
    expect(screen.getByText('/home/user/project')).toBeInTheDocument();
  });

  it('does not show working directory when not set', () => {
    renderCard({ working_dir: undefined });
    expect(screen.queryByText('dir')).not.toBeInTheDocument();
  });

  it('displays initial prompt when set', () => {
    renderCard({ initial_prompt: 'Analyze the codebase' });
    expect(screen.getByText('prompt')).toBeInTheDocument();
    expect(screen.getByText('Analyze the codebase')).toBeInTheDocument();
  });

  it('does not show prompt section when initial_prompt is not set', () => {
    renderCard({ initial_prompt: undefined });
    expect(screen.queryByText('prompt')).not.toBeInTheDocument();
  });

  it('displays tags when present', () => {
    renderCard({ tags: ['deploy', 'production', 'frontend'] });
    expect(screen.getByText('deploy')).toBeInTheDocument();
    expect(screen.getByText('production')).toBeInTheDocument();
    expect(screen.getByText('frontend')).toBeInTheDocument();
  });

  it('does not show tags section when tags is empty', () => {
    renderCard({ tags: [] });
    // No tag elements should be rendered
    expect(screen.queryByText('deploy')).not.toBeInTheDocument();
  });

  it('does not show tags section when tags is undefined', () => {
    renderCard({ tags: undefined });
    expect(screen.queryByText('deploy')).not.toBeInTheDocument();
  });

  it('links to the template edit page', () => {
    renderCard({ template_id: 'tmpl-link-test' });
    const link = screen.getByRole('link');
    expect(link).toHaveAttribute('href', '/templates/tmpl-link-test/edit');
  });
});
