import { Link } from 'react-router';
import { FileQuestion } from 'lucide-react';

export function NotFoundPage() {
  return (
    <div className="flex flex-col items-center justify-center min-h-[60vh] px-4 text-center">
      <div className="text-text-secondary mb-6 opacity-50">
        <FileQuestion size={64} />
      </div>
      <h1 className="text-2xl font-bold text-text-primary mb-2">Page not found</h1>
      <p className="text-sm text-text-secondary max-w-md mb-6">
        The page you're looking for doesn't exist or has been moved.
      </p>
      <Link
        to="/"
        className="px-4 py-2.5 rounded-lg bg-accent-primary text-white text-sm font-medium hover:bg-accent-primary/90 hover:shadow-[0_0_20px_rgba(59,130,246,0.3)] transition-all"
      >
        Go to Command Center
      </Link>
    </div>
  );
}
