import type { ReactNode } from 'react';

export interface CardProps {
  children: ReactNode;
  padding?: number;
  className?: string;
}

export function Card({ children, padding = 16, className = '' }: CardProps) {
  return (
    <div className={`card ${className}`.trim()} style={{ padding }}>
      {children}
    </div>
  );
}
