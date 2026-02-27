import * as React from "react";

interface ButtonProps {
  variant?: "default" | "destructive" | "outline";
  size?: "sm" | "md" | "lg";
  children: React.ReactNode;
}

export function Button({ variant = "default", size = "md", children }: ButtonProps) {
  return <button className={`btn-${variant} btn-${size}`}>{children}</button>;
}
