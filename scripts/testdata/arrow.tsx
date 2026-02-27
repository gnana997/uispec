import * as React from "react";

interface CardProps {
  title: string;
  children?: React.ReactNode;
}

export const Card = ({ title, children }: CardProps) => (
  <div className="card">
    <h2>{title}</h2>
    {children}
  </div>
);
